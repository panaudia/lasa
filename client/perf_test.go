package client

// End-to-end performance benchmarks for the state path THROUGH the MoQ
// transport: client WriteState → QUIC → server ingest → state engine →
// per-subscriber deliverLive → QUIC → client ReadObject. Real server,
// real loopback QUIC connections, one per subscriber. Compare with the
// engine-only numbers in state/perf_test.go to see what the transport
// adds.
//
// This lives in package client (not a _test import) so the reader side
// can subscribe to the state track raw — the same subscribe SyncState
// performs, but handing each message straight to the benchmark with no
// store or polling between the transport and the timestamp.
//
// Run:
//
//	go test ./client -run '^$' -bench 'BenchmarkE2EState' -benchtime 1000x
//
// Fixed iteration counts (-benchtime Nx) are recommended: each b.Run
// re-invocation redials every connection, and the latency benchmark paces
// writes at 1ms each.
//
//   - BenchmarkE2EStateLatency/subs=N: p50/p95/p99 write→deliver latency
//     in µs across N subscriber connections, paced 1k writes/s.
//   - BenchmarkE2EStateThroughput/subs=N: writer at full speed; ns/op is
//     per write, deliveries/s counts sets actually received by all N.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"math"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panaudia/moqtransport"
	"github.com/quic-go/quic-go"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/ident"
	"github.com/panaudia/lasa/server"
	"github.com/panaudia/lasa/state"
	"github.com/panaudia/lasa/wire"
)

const perfKey = "app.perf"

// perfSubCounts returns the subscriber counts to benchmark: default
// {1, 8, 32}, overridable with e.g. LASA_PERF_SUBS=128,256 for capacity
// probing.
func perfSubCounts() []int {
	env := os.Getenv("LASA_PERF_SUBS")
	if env == "" {
		return []int{1, 8, 32}
	}
	var out []int
	for _, part := range strings.Split(env, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 {
			panic("bad LASA_PERF_SUBS: " + env)
		}
		out = append(out, n)
	}
	return out
}

func perfTLS(tb testing.TB) *tls.Config {
	tb.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		tb.Fatal(err)
	}
	tmpl := x509.Certificate{SerialNumber: big.NewInt(1)}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, priv.Public(), priv)
	if err != nil {
		tb.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		tb.Fatal(err)
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		tb.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{server.ALPN}}
}

// nopBackend admits everything and discards ingress: the benchmarks
// exercise only the state path.
type nopBackend struct{}
type nopHandler struct{}

func (nopBackend) Admit(*connect.Resolved) *connect.Rejection { return nil }
func (nopBackend) EntityJoined(string, connect.ResolvedEntity) (server.EntityHandler, error) {
	return nopHandler{}, nil
}
func (nopBackend) EntityLeft(string, string) {}
func (nopBackend) StartSink(string, string, server.SinkWriter) (server.SinkSession, error) {
	return nil, fmt.Errorf("perf: no sinks")
}
func (nopHandler) WritePacket(uint64, []byte) {}

// startPerfSpace runs a real server on loopback QUIC. The engine gets an
// enlarged broadcast window so a briefly slow subscriber lags instead of
// being terminated mid-benchmark.
func startPerfSpace(tb testing.TB) string {
	tb.Helper()
	srv, err := server.New(server.Config{
		SpaceID:         "perf",
		Backend:         nopBackend{},
		AllowUnticketed: true,
		Engine:          state.Config{LogEntries: 1 << 20, LogBytes: 1 << 30},
		RulesFor: func(*connect.Resolved) state.Rules {
			return state.Rules{Set: []string{""}, Clear: []string{""}, Read: []string{""}}
		},
	})
	if err != nil {
		tb.Fatal(err)
	}
	l, err := quic.ListenAddr("localhost:0", perfTLS(tb), &quic.Config{EnableDatagrams: true})
	if err != nil {
		tb.Fatal(err)
	}
	go func() { _ = srv.ServeQUIC(l) }()
	tb.Cleanup(func() { _ = l.Close(); srv.Close() })
	return l.Addr().String()
}

func dialPerf(tb testing.TB, addr, id string) *Client {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Dial(ctx, addr, Config{
		SpaceID:  "perf",
		ClientID: id,
		Entities: []connect.Entity{{ID: "ent-" + id, Name: id}},
		TLS:      &tls.Config{InsecureSkipVerify: true},
	})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = c.Close() })
	return c
}

// subscribeStateRaw performs the state subscribe SyncState performs, but
// delivers each live-lane message straight to h (called on the track's
// read goroutine). Catch-up lane objects (subgroup 1) are skipped: the
// benchmarks only time live delivery.
func subscribeStateRaw(ctx context.Context, tb testing.TB, c *Client, h func(wire.StateMessage)) {
	tb.Helper()
	raw, err := wire.AppendSubscribeParams(nil, &wire.SubscribeParams{Prefixes: []string{""}})
	if err != nil {
		tb.Fatal(err)
	}
	ns := ident.ClientNamespace(c.cfg.SpaceID, c.cfg.ClientID, ident.DirectionSink)
	rt, err := c.session.Subscribe(ctx, ns.Tuple(), ident.TrackState,
		moqtransport.WithSubscribeParameters(moqtransport.KVPList{
			{Type: wire.SubscribeParamState, ValueBytes: raw},
		}))
	if err != nil {
		tb.Fatal(err)
	}
	go func() {
		defer rt.Close()
		for {
			obj, err := rt.ReadObject(ctx)
			if err != nil {
				return
			}
			msg, err := wire.ParseStateMessage(obj.Payload, wire.Downstream)
			if err != nil {
				return
			}
			if obj.SubGroupID != 1 {
				h(msg)
			}
		}
	}()
}

// warmUp writes probe sets until every subscriber has seen one, closing
// the subscribe-registration race before measurement starts.
func warmUp(b *testing.B, ctx context.Context, w *Client, warmSeen []atomic.Bool) {
	b.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if err := w.WriteState(ctx, wire.Set{Key: perfKey + ".warm", Value: []byte{1}}); err != nil {
			b.Fatal(err)
		}
		all := true
		for i := range warmSeen {
			all = all && warmSeen[i].Load()
		}
		if all {
			return
		}
		if time.Now().After(deadline) {
			b.Fatal("timeout warming up subscribers")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func awaitCount(b *testing.B, c *atomic.Uint64, want uint64, what string) {
	b.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for c.Load() < want {
		if time.Now().After(deadline) {
			b.Fatalf("timeout: %s (%d/%d)", what, c.Load(), want)
		}
		time.Sleep(50 * time.Microsecond)
	}
}

// BenchmarkE2EStateLatency: paced writes carrying their send time; every
// subscriber connection stamps receipt; percentiles across all samples.
func BenchmarkE2EStateLatency(b *testing.B) {
	// Write pacing: default 1ms (1k writes/s), overridable with e.g.
	// LASA_PERF_INTERVAL_MS=50 to probe latency at realistic state rates.
	writeInterval := time.Millisecond
	if env := os.Getenv("LASA_PERF_INTERVAL_MS"); env != "" {
		ms, err := strconv.Atoi(env)
		if err != nil || ms < 1 {
			b.Fatal("bad LASA_PERF_INTERVAL_MS: " + env)
		}
		writeInterval = time.Duration(ms) * time.Millisecond
	}
	for _, subs := range perfSubCounts() {
		b.Run(fmt.Sprintf("subs=%d", subs), func(b *testing.B) {
			addr := startPerfSpace(b)
			writer := dialPerf(b, addr, "w")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			base := time.Now()
			var delivered atomic.Uint64
			warmSeen := make([]atomic.Bool, subs)
			samples := make([][]time.Duration, subs)
			for i := 0; i < subs; i++ {
				r := dialPerf(b, addr, fmt.Sprintf("r%d", i))
				i := i
				subscribeStateRaw(ctx, b, r, func(m wire.StateMessage) {
					s, ok := m.(wire.Set)
					if !ok {
						return
					}
					switch s.Key {
					case perfKey + ".warm":
						warmSeen[i].Store(true)
					case perfKey:
						sent := time.Duration(binary.LittleEndian.Uint64(s.Value))
						samples[i] = append(samples[i], time.Since(base)-sent)
						delivered.Add(1)
					}
				})
			}
			warmUp(b, ctx, writer, warmSeen)

			start := time.Now()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if d := time.Until(start.Add(time.Duration(i) * writeInterval)); d > 0 {
					time.Sleep(d)
				}
				val := make([]byte, 32)
				binary.LittleEndian.PutUint64(val, uint64(time.Since(base)))
				if err := writer.WriteState(ctx, wire.Set{Key: perfKey, Value: val}); err != nil {
					b.Fatal(err)
				}
			}
			awaitCount(b, &delivered, uint64(b.N)*uint64(subs), "deliveries")
			b.StopTimer()
			cancel()
			var all []time.Duration
			for _, s := range samples {
				all = append(all, s...)
			}
			sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
			b.ReportMetric(float64(pct(all, 0.50).Microseconds()), "p50-µs")
			b.ReportMetric(float64(pct(all, 0.95).Microseconds()), "p95-µs")
			b.ReportMetric(float64(pct(all, 0.99).Microseconds()), "p99-µs")
		})
	}
}

// BenchmarkE2EStateThroughput: writer at full speed, self-throttled so no
// subscriber falls more than `slack` writes behind. Timing stops after
// every subscriber has received everything.
func BenchmarkE2EStateThroughput(b *testing.B) {
	for _, subs := range perfSubCounts() {
		b.Run(fmt.Sprintf("subs=%d", subs), func(b *testing.B) {
			addr := startPerfSpace(b)
			writer := dialPerf(b, addr, "w")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var total atomic.Uint64 // across subs; min tracked separately
			warmSeen := make([]atomic.Bool, subs)
			received := make([]atomic.Uint64, subs)
			for i := 0; i < subs; i++ {
				r := dialPerf(b, addr, fmt.Sprintf("r%d", i))
				i := i
				subscribeStateRaw(ctx, b, r, func(m wire.StateMessage) {
					s, ok := m.(wire.Set)
					if !ok {
						return
					}
					switch s.Key {
					case perfKey + ".warm":
						warmSeen[i].Store(true)
					case perfKey:
						received[i].Add(1)
						total.Add(1)
					}
				})
			}
			warmUp(b, ctx, writer, warmSeen)

			val := make([]byte, 32)
			const slack = 8192
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := writer.WriteState(ctx, wire.Set{Key: perfKey, Value: val}); err != nil {
					b.Fatal(err)
				}
				if i&255 == 0 {
					for minCount(received)+slack < uint64(i) {
						time.Sleep(50 * time.Microsecond)
					}
				}
			}
			awaitCount(b, &total, uint64(b.N)*uint64(subs), "deliveries")
			b.StopTimer()
			b.ReportMetric(float64(b.N*subs)/b.Elapsed().Seconds(), "deliveries/s")
			cancel()
		})
	}
}

func minCount(counts []atomic.Uint64) uint64 {
	m := uint64(math.MaxUint64)
	for i := range counts {
		if c := counts[i].Load(); c < m {
			m = c
		}
	}
	return m
}

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[int(p*float64(len(sorted)-1))]
}
