package conformance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/panaudia/lasa/client"
	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// TestUplinkObserver drives the reference Go client against the
// observer on both transports and closes the client-side sender rows
// from the wire: one gapless counter with pose-only and empty packets
// counted (C-5.1-1, C-5.3-2), no reserved or ts bits (C-5.1-6,
// C-5.1-14), redundancy within the declaration (C-4.2-10), copies that
// reference audio-bearing packets and repeat them exactly (C-5.1-13,
// C-5.1-12), and a reconnect restarting at zero (C-5.2-1). The Go
// client is the implementation under test here, in process; other
// clients arrive through the driver shim (plan §2.4).
func TestUplinkObserver(t *testing.T) {
	if externalOnly() {
		t.Skip("the observer hosts its own space")
	}
	for _, tr := range []Transport{TransportQUIC, TransportWebTransport} {
		t.Run(string(tr), func(t *testing.T) {
			obs, err := StartObserver(tr, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(obs.Close)
			f := obs.Fixture

			entities := []connect.Entity{{ID: "up-red", Name: "R", Redundancy: 2}, {ID: "up-plain", Name: "P"}}
			c := mustDial(t, f, party{clientID: "up-c", entities: entities})
			publishPattern(t, c, 300)

			var red, plain TrackReport
			waitCond(t, "300 packets observed on both tracks", func() bool {
				rr, pr := obs.Reports("up-red"), obs.Reports("up-plain")
				if len(rr) == 0 || len(pr) == 0 {
					return false
				}
				red, plain = rr[0], pr[0]
				return red.Last >= 299 && plain.Last >= 299
			})
			t.Log(red.Summary())
			t.Log(plain.Summary())

			for _, r := range []TrackReport{red, plain} {
				Check(t, "C-5.1-1", r.First == 0 && r.Dups == 0 && r.Backwards == 0 && r.Gaps*100 <= r.Received,
					"%s: first %d, %d gaps, %d dups, %d backwards over %d packets", r.EntityID, r.First, r.Gaps, r.Dups, r.Backwards, r.Received)
				Check(t, "C-5.1-6", r.UnknownFlags == 0, "%s: %d packets with reserved bits set", r.EntityID, r.UnknownFlags)
				Check(t, "C-5.1-14", r.UnknownFlags == 0, "%s: %d packets with the ts bit set on the uplink", r.EntityID, r.UnknownFlags)
				Check(t, "C-5.3-2", r.Empty > 0 && r.PoseOnly > 0 && r.Malformed == 0,
					"%s: empty %d, pose-only %d, malformed %d: the pattern's flags 0x00 and pose-only packets must arrive and count", r.EntityID, r.Empty, r.PoseOnly, r.Malformed)
			}
			report.record("C-5.1-1", OutcomePass, "uplink on "+f.Name+": "+red.Summary())
			Check(t, "C-4.2-10", red.OffsetOverDeclaration == 0 && plain.Redundant == 0 && red.Redundant > 0,
				"declared 2: over %d, redundant %d; undeclared: redundant %d", red.OffsetOverDeclaration, red.Redundant, plain.Redundant)
			Check(t, "C-5.1-13", red.RefNotAudio == 0, "%d copies referenced a packet without audio", red.RefNotAudio)
			Check(t, "C-5.1-12", red.RefMismatch == 0 && red.Redundant > red.RefUnseen,
				"%d copies differ from the referenced frame (%d unseen of %d)", red.RefMismatch, red.RefUnseen, red.Redundant)
			report.record("C-5.1-12", OutcomePass, "uplink copies on "+f.Name+": "+red.Summary())

			// Reconnect: a new track instance, ids from zero.
			_ = c.Close()
			c2 := mustDial(t, f, party{clientID: "up-c", entities: entities})
			publishPattern(t, c2, 50)
			var second TrackReport
			waitCond(t, "second instance observed", func() bool {
				rr := obs.Reports("up-red")
				if len(rr) < 2 {
					return false
				}
				second = rr[1]
				return second.Last >= 49
			})
			Check(t, "C-5.2-1", second.Instance == 2 && second.First == 0, "reconnect instance %d started at seq %d", second.Instance, second.First)
			Pass(t, "C-4.5-2", "uplink ids restarted with the new session")
		})
	}
}

// publishPattern sends n packets on every entity: audio and pose by
// default, pose-only at seq%10==3, flags 0x00 at seq%10==7, so the
// redundancy emitter meets references with and without audio.
func publishPattern(t *testing.T, c *client.Client, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	var pubs []*client.EntityPublisher
	for _, id := range []string{"up-red", "up-plain"} {
		p, err := c.Entity(ctx, id) // returns once the space has subscribed
		if err != nil {
			t.Fatal(err)
		}
		pubs = append(pubs, p)
	}
	tk := time.NewTicker(5 * time.Millisecond)
	defer tk.Stop()
	for seq := uint64(0); seq < uint64(n); seq++ {
		<-tk.C
		for _, p := range pubs {
			pkt := wire.MonoObjectPacket{}
			switch seq % 10 {
			case 3:
				pkt.Pose = &wire.Pose{X: float32(seq)}
			case 7:
			default:
				pkt.Pose = &wire.Pose{X: float32(seq)}
				pkt.Audio = []byte{0xA0, byte(seq), byte(seq >> 8), 0xC0}
			}
			if err := p.WriteMonoObject(seq, &pkt); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestUplinkLockstep covers C-4.2-12 and C-5.2-4 from the observer: on
// a lockstep connection every member's counter runs together, every
// member emits at every frame, and a reconnect restarts them together.
// The Go client leaves the counter to its caller, so publishPattern's
// one seq per tick is the in-step sender here.
func TestUplinkLockstep(t *testing.T) {
	if externalOnly() {
		t.Skip("the observer hosts its own space")
	}
	for _, tr := range []Transport{TransportQUIC, TransportWebTransport} {
		t.Run(string(tr), func(t *testing.T) {
			obs, err := StartObserver(tr, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(obs.Close)
			f := obs.Fixture
			entities := []connect.Entity{{ID: "up-red", Name: "R", Redundancy: 2}, {ID: "up-plain", Name: "P"}}
			cfg := configFor(t, f, party{clientID: "lock-c", entities: entities})
			cfg.Lockstep = true
			c, err := dial(t, f, cfg)
			if err != nil {
				t.Fatal(err)
			}
			publishPattern(t, c, 300)
			var red, plain TrackReport
			waitCond(t, "300 packets on both members", func() bool {
				rr, pr := obs.Reports("up-red"), obs.Reports("up-plain")
				if len(rr) == 0 || len(pr) == 0 {
					return false
				}
				red, plain = rr[len(rr)-1], pr[len(pr)-1]
				return red.Last >= 299 && plain.Last >= 299
			})
			t.Log(red.Summary())
			if red.Lockstep == nil || plain.Lockstep == nil {
				t.Fatal("the observer did not see the lockstep declaration")
			}
			Check(t, "C-4.2-12", red.Lockstep.Count == 2 && plain.Lockstep.Count == 2 && red.Lockstep.Index != plain.Lockstep.Index,
				"set stamped as %+v and %+v", red.Lockstep, plain.Lockstep)
			dense := red.Received == plain.Received && red.Gaps == 0 && plain.Gaps == 0 && red.Empty > 0
			Check(t, "C-5.2-4", dense && red.Lockstep.MaxSpread <= 2 && red.First == 0 && plain.First == 0,
				"in step: received %d/%d, gaps %d/%d, empty %d, max spread %d ids", red.Received, plain.Received, red.Gaps, plain.Gaps, red.Empty, red.Lockstep.MaxSpread)
			report.record("C-5.2-4", OutcomePass, fmt.Sprintf("%s: max spread %d ids over 300 frames", f.Name, red.Lockstep.MaxSpread))
			_ = c.Close()
			c2, err := dial(t, f, cfg)
			if err != nil {
				t.Fatal(err)
			}
			publishPattern(t, c2, 50)
			waitCond(t, "second instance in step from zero", func() bool {
				rr, pr := obs.Reports("up-red"), obs.Reports("up-plain")
				if len(rr) < 2 || len(pr) < 2 {
					return false
				}
				a, b := rr[1], pr[1]
				return a.Last >= 49 && b.Last >= 49 && a.First == 0 && b.First == 0 && a.Lockstep != nil && a.Lockstep.MaxSpread <= 2
			})
		})
	}
}
