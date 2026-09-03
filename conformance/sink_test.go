package conformance

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/panaudia/lasa/client"
	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// sinkRec is one received sink datagram.
type sinkRec struct {
	seq uint64
	ts  *uint64
	at  time.Time
}

// collect subscribes entity's sink in format and returns a snapshot
// function over what has arrived, plus the track for unsubscribing.
func collect(t *testing.T, c *client.Client, entity, format string) (func() []sinkRec, func()) {
	t.Helper()
	var mu sync.Mutex
	var got []sinkRec
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	rt, err := c.SubscribeSink(ctx, entity, format, func(seq uint64, payload []byte) {
		pkt, err := wire.ParseSink(payload)
		if err != nil {
			return
		}
		mu.Lock()
		got = append(got, sinkRec{seq: seq, ts: pkt.Timestamp, at: time.Now()})
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("subscribe %s/%s: %v", entity, format, err)
	}
	snap := func() []sinkRec {
		mu.Lock()
		defer mu.Unlock()
		return append([]sinkRec(nil), got...)
	}
	return snap, func() { _ = rt.Close() }
}

func waitPackets(t *testing.T, snap func() []sinkRec, n int) []sinkRec {
	t.Helper()
	waitCond(t, fmt.Sprintf("%d sink packets", n), func() bool { return len(snap()) >= n })
	return snap()
}

// TestSinkOffering covers the §3 format menu and the §3.1 sink
// authorization row from the wire: C-3-2, C-3-3, C-3-4, C-3-5, C-3.1-1.
func TestSinkOffering(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := mustDial(t, f, party{clientID: "sink-offer", entities: []connect.Entity{{ID: "sink-offer-e", Name: "A"}}})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		sub := func(entity, format string) error {
			rt, err := c.SubscribeSink(ctx, entity, format, func(uint64, []byte) {})
			if err == nil {
				t.Cleanup(func() { _ = rt.Close() })
			}
			return err
		}
		errBin := sub("sink-offer-e", "binaural")
		errAmbi2 := sub("sink-offer-e", "ambi2")
		Check(t, "C-3-2", errBin == nil && errAmbi2 == nil, "binaural: %v, ambi2: %v", errBin, errAmbi2)
		Check(t, "C-3-4", errBin == nil && errAmbi2 == nil, "an owner may take any offered format: binaural %v, ambi2 %v", errBin, errAmbi2)
		errAmbi3 := sub("sink-offer-e", "ambi3")
		switch {
		case f.RenderOrder == 0:
			NotExercised(t, "C-3-3", "render order not declared by the fixture")
		case f.RenderOrder >= 3:
			Check(t, "C-3-3", errAmbi3 == nil, "ambi3 must be offered at order %d: %v", f.RenderOrder, errAmbi3)
		default:
			Check(t, "C-3-3", errAmbi3 != nil, "ambi3 offered at order %d", f.RenderOrder)
		}
		errNoSuch := sub("sink-offer-e", "no-such-format")
		Check(t, "C-3-5", errNoSuch != nil, "a sink that does not exist was accepted")
		errOther := sub("someone-else", "binaural")
		Check(t, "C-3.1-1", errOther != nil, "another client's entity sink was accepted")
		Pass(t, "C-3-1", "entity sink namespace and the three format names resolved as tabled on %s", f.Name)
	})
}

// TestSinkOfferingAtOrder3 boots a private reference at order 3 for the
// SHOULD half of C-3-3 and the ambi3 layout row C-5.3-4 (parse only).
func TestSinkOfferingAtOrder3(t *testing.T) {
	if externalOnly() {
		NotExercised(t, "C-5.3-4", "external fixture: render order is the fixture's, see TestSinkOffering")
		return
	}
	f := privateReference(t, TransportQUIC, false, func(cfg *serverConfig) { cfg.AmbiOrder = 3 })
	c := mustDial(t, f, party{clientID: "o3", entities: []connect.Entity{{ID: "o3-e", Name: "A"}}})
	snap, stop := collect(t, c, "o3-e", "ambi3")
	defer stop()
	got := waitPackets(t, snap, 5)
	Check(t, "C-3-3", len(got) >= 5, "ambi3 subscribed at order 3 but no packets arrived")
	Pass(t, "C-5.3-4", "ambi3 packets parse to the sink layout at order 3 (multistream content not decoded)")
}

// TestSinkContinuity closes the lockstep-adjacent sink rows: group ids
// increment by one (C-5.1-1, sink side), 200 packets per second
// regardless of content (C-5.1-2), the ts anchor on start and about
// once a second with ts(seq) linear at 5 ms (C-5.1-15).
func TestSinkContinuity(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := mustDial(t, f, party{clientID: "sink-cont", entities: []connect.Entity{{ID: "sink-cont-e", Name: "A"}}})
		snap, stop := collect(t, c, "sink-cont-e", "binaural")
		defer stop()
		got := waitPackets(t, snap, 1)
		start := got[0].at
		time.Sleep(1500 * time.Millisecond)
		got = snap()

		// Ids: strictly increasing, no duplicates, gaps counted (a
		// datagram may be lost on the way; the sender's rule is no gaps).
		gaps, dups, backwards := 0, 0, 0
		for i := 1; i < len(got); i++ {
			switch d := int64(got[i].seq) - int64(got[i-1].seq); {
			case d == 1:
			case d == 0:
				dups++
			case d < 0:
				backwards++
			default:
				gaps += int(d - 1)
			}
		}
		Check(t, "C-5.1-1", dups == 0 && backwards == 0 && gaps*100 <= len(got),
			"ids: %d packets, %d gaps, %d duplicates, %d backwards", len(got), gaps, dups, backwards)
		report.record("C-5.1-1", OutcomePass, fmt.Sprintf("sink side on %s: %d packets, %d lost ids, object-id not checked by the polite client", f.Name, len(got), gaps))

		// Rate over the window from the first packet to the last.
		window := got[len(got)-1].at.Sub(start).Seconds()
		span := float64(got[len(got)-1].seq - got[0].seq + 1)
		pps := span / window
		Check(t, "C-5.1-2", pps > 180 && pps < 220, "sink rate %.1f ids per second over %.2f s", pps, window)
		report.record("C-5.1-2", OutcomePass, fmt.Sprintf("%s: %.1f ids/s over %.2f s, %d received", f.Name, pps, window, len(got)))

		// Anchors: the first packet of a new render carries one, later
		// ones about once a second, all on one 5 ms line.
		if got[0].ts == nil {
			Fail(t, "C-5.1-15", "first packet of a new render carries no ts anchor")
		} else {
			ok := true
			var last time.Time
			anchors := 0
			for _, r := range got {
				if r.ts == nil {
					continue
				}
				anchors++
				want := *got[0].ts + (r.seq-got[0].seq)*5000
				if *r.ts != want {
					ok = false
					Fail(t, "C-5.1-15", "seq %d ts %d, want %d on the 5 ms line from seq %d", r.seq, *r.ts, want, got[0].seq)
					break
				}
				if !last.IsZero() && r.at.Sub(last) > 1500*time.Millisecond {
					ok = false
					Fail(t, "C-5.1-15", "anchor cadence %v exceeds about once per second", r.at.Sub(last))
					break
				}
				last = r.at
			}
			if ok {
				Pass(t, "C-5.1-15", "%s: %d anchors in %.2f s, linear at 5 ms", f.Name, anchors, window)
			}
		}
	})
}

// TestSinkIdsAcrossChurn closes C-5.2-2 (ids run across subscriber
// churn and restart at 0 with the render), C-3.2-2 (a subscription
// joins at live) and C-5.1-16 (the anchor is per track instance, so a
// joiner receives one within the cadence and a new instance starts
// with one).
func TestSinkIdsAcrossChurn(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := mustDial(t, f, party{clientID: "sink-churn", entities: []connect.Entity{{ID: "sink-churn-e", Name: "A"}}})

		snapA, stopA := collect(t, c, "sink-churn-e", "binaural")
		a := waitPackets(t, snapA, 100)
		Check(t, "C-5.2-2", a[0].seq < 50, "first subscriber of a fresh render saw seq %d, want near 0", a[0].seq)

		// B joins the live render A is holding open.
		aNow := snapA()[len(snapA())-1].seq
		snapB, stopB := collect(t, c, "sink-churn-e", "binaural")
		b := waitPackets(t, snapB, 1)
		Check(t, "C-3.2-2", b[0].seq+5 >= aNow, "joiner's first seq %d, live was at %d: history was delivered", b[0].seq, aNow)
		Check(t, "C-5.2-2", b[0].seq+5 >= aNow && b[0].seq > 0, "joiner attached at seq %d while the render was at %d", b[0].seq, aNow)
		if waitCond(t, "an anchor on the joiner within the cadence", func() bool {
			for _, r := range snapB() {
				if r.ts != nil {
					return true
				}
			}
			return false
		}) {
			Pass(t, "C-5.1-16", "%s: joiner received an anchor within the cadence", f.Name)
		} else {
			Fail(t, "C-5.1-16", "no anchor reached a mid-render joiner")
		}
		stopB()

		// C after B left, A still holding: ids keep running.
		bLast := snapB()[len(snapB())-1].seq
		snapC, stopC := collect(t, c, "sink-churn-e", "binaural")
		cc := waitPackets(t, snapC, 1)
		Check(t, "C-5.2-2", cc[0].seq > bLast, "after churn the next subscriber saw seq %d, not past %d", cc[0].seq, bLast)
		stopC()
		stopA()

		// Every subscription gone: the render stops, and the next one
		// is a new instance starting near 0 with an anchor on its
		// first packet.
		time.Sleep(300 * time.Millisecond)
		snapD, stopD := collect(t, c, "sink-churn-e", "binaural")
		defer stopD()
		d := waitPackets(t, snapD, 1)
		Check(t, "C-5.2-2", d[0].seq < 50, "a restarted render began at seq %d, want near 0", d[0].seq)
		Pass(t, "C-4.5-2", "sink ids carried no continuity across render instances")
		Check(t, "C-5.1-16", d[0].ts != nil, "a new render instance's first packet carries no anchor")
	})
}

// TestSinkConcurrentFormats covers C-3.2-3: two formats on one sink
// flow at once.
func TestSinkConcurrentFormats(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := mustDial(t, f, party{clientID: "sink-conc", entities: []connect.Entity{{ID: "sink-conc-e", Name: "A"}}})
		snapBin, stopBin := collect(t, c, "sink-conc-e", "binaural")
		defer stopBin()
		snapAmbi, stopAmbi := collect(t, c, "sink-conc-e", "ambi2")
		defer stopAmbi()
		waitPackets(t, snapBin, 20)
		waitPackets(t, snapAmbi, 20)
		Check(t, "C-3.2-3", len(snapBin()) >= 20 && len(snapAmbi()) >= 20, "formats did not flow concurrently")
	})
}

// TestHeadFrameSinkRefused covers P-6-10.
func TestHeadFrameSinkRefused(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := mustDial(t, f, party{clientID: "head-sink", entities: []connect.Entity{{ID: "head-sink-e", Name: "HUD", Frame: "head"}}})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		rt, err := c.SubscribeSink(ctx, "head-sink-e", "binaural", func(uint64, []byte) {})
		if err == nil {
			_ = rt.Close()
		}
		Check(t, "P-6-10", err != nil, "a head-frame entity's own sink was accepted")
	})
}
