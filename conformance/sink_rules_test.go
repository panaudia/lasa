package conformance

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/panaudia/moqtransport"

	"github.com/panaudia/lasa/connect"
	"github.com/panaudia/lasa/wire"
)

// TestSinkRedundancy covers C-4.2-9 (the space SHOULD honour a sink
// redundancy request) and C-4.2-11 (it never exceeds the declaration).
func TestSinkRedundancy(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		c := mustDial(t, f, party{clientID: "sred-c", entities: []connect.Entity{{ID: "sred-2", Name: "R", Redundancy: 2}, {ID: "sred-0", Name: "P"}}})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		offsets := func(entity string) func() (n, red, over int) {
			var mu sync.Mutex
			var cnt, redundant, overDecl int
			decl := 0
			if entity == "sred-2" {
				decl = 2
			}
			rt, err := c.SubscribeSink(ctx, entity, "binaural", func(_ uint64, payload []byte) {
				pkt, err := wire.ParseSink(payload)
				if err != nil {
					return
				}
				mu.Lock()
				cnt++
				if pkt.Redundancy != nil {
					redundant++
					if int(pkt.Redundancy.Offset) > decl {
						overDecl++
					}
				}
				mu.Unlock()
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = rt.Close() })
			return func() (int, int, int) { mu.Lock(); defer mu.Unlock(); return cnt, redundant, overDecl }
		}
		two, none := offsets("sred-2"), offsets("sred-0")
		waitCond(t, "100 packets on both", func() bool { a, _, _ := two(); b, _, _ := none(); return a >= 100 && b >= 100 })
		n2, r2, o2 := two()
		_, r0, o0 := none()
		Check(t, "C-4.2-11", o2 == 0 && o0 == 0 && r0 == 0, "offsets over the declaration: declared-2 %d, undeclared %d (redundant %d)", o2, o0, r0)
		if r2 > 0 {
			Pass(t, "C-4.2-9", "%s: %d of %d sink packets carried a copy at offset 2", f.Name, r2, n2)
		} else {
			NotExercised(t, "C-4.2-9", "the space declined the sink redundancy request (policy)")
		}
	})
}

// TestSinkEndsOnDeparture covers C-3.2-4: when the entity departs, a
// live subscription to its sink is closed with SUBSCRIBE_DONE. Needs a
// second connection allowed to hold the sink, so a producer, so
// ticketed.
func TestSinkEndsOnDeparture(t *testing.T) {
	eachFixture(t, func(t *testing.T, f *Fixture) {
		if !f.Ticketed {
			NotExercised(t, "C-3.2-4", "unticketed fixture: no role may hold another entity's sink")
			return
		}
		owner := mustDial(t, f, party{clientID: "dep-owner", entities: []connect.Entity{{ID: "dep-e", Name: "D"}}})
		producer := mustDial(t, f, party{clientID: "dep-producer", entities: []connect.Entity{}, roles: []string{"producer"}})
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
		defer cancel()
		got := 0
		rt, err := producer.SubscribeSink(ctx, "dep-e", "binaural", func(uint64, []byte) { got++ })
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
		_ = owner.Close()
		_, err = rt.ReadObject(ctx)
		var done *moqtransport.ErrSubscribeDone
		Check(t, "C-3.2-4", errors.As(err, &done), "after the entity departed the sink subscription ended with %v, want SUBSCRIBE_DONE", err)
	})
}
