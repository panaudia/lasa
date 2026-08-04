package state

// Performance benchmarks for the state path:
//
//	Conn.Write → actor → store/broadcast log → Subscription.LiveNext →
//	wire.AppendStateMessage (the encode the server's deliverLive performs)
//
// Everything but the MoQ transport itself, so the numbers are an upper
// bound on what a real deployment sees.
//
// Run all of them:
//
//	go test ./state -run '^$' -bench 'BenchmarkState' -benchtime 2s
//
// The latency benchmark paces writes (writeInterval), so each iteration
// costs ~1ms of wall clock; give it an iteration count rather than a
// duration for a decent sample size:
//
//	go test ./state -run '^$' -bench 'BenchmarkStateLatency' -benchtime 3000x
//
// Interpreting the numbers:
//
//   - BenchmarkStateWrite / WriteParallel: ns/op is the serialized cost of
//     one client Set through the actor. 1/ns-per-op = max sustainable
//     aggregate write rate (all clients combined), independent of readers.
//   - BenchmarkStateFanout/subs=N: ns/op is per *write*; deliveries/s is
//     writes × subscribers actually drained and encoded per second. For a
//     mesh of N clients each writing pose at R Hz and subscribed to
//     everything, you need N×R writes/s and N²×R deliveries/s.
//   - BenchmarkStateLatency/subs=N: p50/p95/p99 write→deliver latency in
//     µs across all subscribers, under a paced (non-saturating) load.

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panaudia/lasa/wire"
)

const benchValueLen = 32 // a pose-sized value

// newBenchEngine uses the real clock (latency needs it) and a broadcast
// window big enough that a temporarily slow drain lags instead of dying.
func newBenchEngine(b *testing.B) *Engine {
	b.Helper()
	e := New(Config{LogEntries: 1 << 20, LogBytes: 1 << 30})
	b.Cleanup(e.Close)
	return e
}

// attachDrained registers a reader and consumes its (empty) catch-up lane
// so only live-lane traffic remains.
func attachDrained(b *testing.B, e *Engine, id string) *Subscription {
	b.Helper()
	c, err := e.Register(id, allAccess)
	if err != nil {
		b.Fatal(err)
	}
	s, err := e.Attach(c, []string{""}, nil)
	if err != nil {
		b.Fatal(err)
	}
	for {
		if _, err := s.CatchUpNext(context.Background()); err == io.EOF {
			return s
		} else if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStateWrite measures one Set through the actor with no
// subscribers: the raw ingest ceiling.
func BenchmarkStateWrite(b *testing.B) {
	e := newBenchEngine(b)
	c, err := e.Register("w", allAccess)
	if err != nil {
		b.Fatal(err)
	}
	keys := benchKeys(256)
	val := make([]byte, benchValueLen)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.Write(wire.Set{Key: keys[i%len(keys)], Value: val}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStateWriteParallel is the same ceiling with many goroutines
// contending for the actor, as concurrent connections would.
func BenchmarkStateWriteParallel(b *testing.B) {
	e := newBenchEngine(b)
	keys := benchKeys(256)
	val := make([]byte, benchValueLen)
	var next atomic.Uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := next.Add(1)
		c, err := e.Register(fmt.Sprintf("w%d", id), allAccess)
		if err != nil {
			b.Error(err)
			return
		}
		i := int(id)
		for pb.Next() {
			i++
			if err := c.Write(wire.Set{Key: keys[i%len(keys)], Value: val}); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

// BenchmarkStateWriteBatch: k ops per Conn.Write call — one actor
// round-trip amortized over k ops, the shape of an edge node forwarding
// its clients' writes to a singleton state service. ns/op is per op.
func BenchmarkStateWriteBatch(b *testing.B) {
	for _, batch := range []int{16, 64} {
		b.Run(fmt.Sprintf("batch=%d", batch), func(b *testing.B) {
			e := newBenchEngine(b)
			c, err := e.Register("w", allAccess)
			if err != nil {
				b.Fatal(err)
			}
			keys := benchKeys(256)
			val := make([]byte, benchValueLen)
			msgs := make([]wire.StateMessage, batch)
			b.ResetTimer()
			for i := 0; i < b.N; i += batch {
				for j := 0; j < batch; j++ {
					msgs[j] = wire.Set{Key: keys[(i+j)%len(keys)], Value: val}
				}
				if err := c.Write(msgs...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkStateFanout: one writer at full speed, N subscribers each
// draining and wire-encoding every op. The writer self-throttles to half
// the broadcast window so slow drains lag rather than terminate. Timing
// stops only after every subscriber has drained everything, so ns/op and
// deliveries/s reflect completed delivery, not just ingest.
func BenchmarkStateFanout(b *testing.B) {
	for _, subs := range []int{1, 8, 32, 128} {
		b.Run(fmt.Sprintf("subs=%d", subs), func(b *testing.B) {
			e := newBenchEngine(b)
			writer, err := e.Register("w", allAccess)
			if err != nil {
				b.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var wg sync.WaitGroup
			received := make([]atomic.Uint64, subs)
			for i := 0; i < subs; i++ {
				s := attachDrained(b, e, fmt.Sprintf("r%d", i))
				wg.Add(1)
				go func(i int, s *Subscription) {
					defer wg.Done()
					buf := make([]byte, 0, 256)
					for {
						m, err := s.LiveNext(ctx)
						if err != nil {
							return
						}
						buf = buf[:0]
						buf, _ = wire.AppendStateMessage(buf, m, wire.Downstream)
						if _, ok := m.(wire.Set); ok {
							received[i].Add(1)
						}
					}
				}(i, s)
			}
			val := make([]byte, benchValueLen)
			const slack = 1 << 19 // half the broadcast window
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := writer.Write(wire.Set{Key: "app.pose.w", Value: val}); err != nil {
					b.Fatal(err)
				}
				if i&1023 == 0 {
					for minReceived(received)+slack < uint64(i) {
						time.Sleep(50 * time.Microsecond)
					}
				}
			}
			for minReceived(received) < uint64(b.N) {
				time.Sleep(50 * time.Microsecond)
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N*subs)/b.Elapsed().Seconds(), "deliveries/s")
			cancel()
			wg.Wait()
		})
	}
}

func minReceived(counts []atomic.Uint64) uint64 {
	m := uint64(math.MaxUint64)
	for i := range counts {
		if c := counts[i].Load(); c < m {
			m = c
		}
	}
	return m
}

// BenchmarkStateLatency measures write→deliver latency under a paced,
// non-saturating load (writeInterval per write ≈ 50 pose clients at
// 20 Hz). Each value carries its write timestamp; every subscriber
// records the delta on receipt, and p50/p95/p99 are computed across all
// subscribers' samples.
func BenchmarkStateLatency(b *testing.B) {
	const writeInterval = time.Millisecond
	for _, subs := range []int{1, 32, 128} {
		b.Run(fmt.Sprintf("subs=%d", subs), func(b *testing.B) {
			e := newBenchEngine(b)
			writer, err := e.Register("w", allAccess)
			if err != nil {
				b.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			base := time.Now()
			var wg sync.WaitGroup
			var delivered atomic.Uint64
			samples := make([][]time.Duration, subs)
			for i := 0; i < subs; i++ {
				s := attachDrained(b, e, fmt.Sprintf("r%d", i))
				wg.Add(1)
				go func(i int, s *Subscription) {
					defer wg.Done()
					buf := make([]byte, 0, 256)
					for {
						m, err := s.LiveNext(ctx)
						if err != nil {
							return
						}
						if set, ok := m.(wire.Set); ok {
							sent := time.Duration(binary.LittleEndian.Uint64(set.Value))
							samples[i] = append(samples[i], time.Since(base)-sent)
							delivered.Add(1)
						}
						buf = buf[:0]
						buf, _ = wire.AppendStateMessage(buf, m, wire.Downstream)
					}
				}(i, s)
			}
			start := time.Now()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if d := time.Until(start.Add(time.Duration(i) * writeInterval)); d > 0 {
					time.Sleep(d)
				}
				// Fresh buffer per write: the engine stores the value
				// slice by reference (immutable by convention).
				val := make([]byte, benchValueLen)
				binary.LittleEndian.PutUint64(val, uint64(time.Since(base)))
				if err := writer.Write(wire.Set{Key: "app.pose.w", Value: val}); err != nil {
					b.Fatal(err)
				}
			}
			want := uint64(b.N) * uint64(subs)
			for delivered.Load() < want {
				time.Sleep(50 * time.Microsecond)
			}
			b.StopTimer()
			cancel()
			wg.Wait()
			var all []time.Duration
			for _, s := range samples {
				all = append(all, s...)
			}
			sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
			b.ReportMetric(float64(percentile(all, 0.50).Microseconds()), "p50-µs")
			b.ReportMetric(float64(percentile(all, 0.95).Microseconds()), "p95-µs")
			b.ReportMetric(float64(percentile(all, 0.99).Microseconds()), "p99-µs")
		})
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p * float64(len(sorted)-1))
	return sorted[i]
}

func benchKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("app.pose.client%03d", i)
	}
	return keys
}
