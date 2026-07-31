package state

import (
	"sync"
	"testing"

	"github.com/panaudia/lasa/wire"
)

func TestObserverReplayAndNotify(t *testing.T) {
	e := New(Config{})
	defer e.Close()

	// Pre-existing state to replay.
	if err := e.Write(wire.Set{Key: "mod.block.a", Value: []byte("1")},
		wire.Set{Key: "other.key", Value: []byte("x")}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	type ev struct {
		key     string
		val     string
		cleared bool
	}
	var got []ev
	err := e.Observe([]string{"mod.block."}, func(k string, v []byte, cleared bool) {
		mu.Lock()
		got = append(got, ev{k, string(v), cleared})
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}

	// Replay delivered the live matching key, not the other one.
	mu.Lock()
	if len(got) != 1 || got[0].key != "mod.block.a" || got[0].val != "1" {
		t.Fatalf("replay: %+v", got)
	}
	mu.Unlock()

	// Live notify: set and clear, in order; non-matching keys silent.
	if err := e.Write(wire.Set{Key: "mod.block.b", Value: []byte("2")},
		wire.Set{Key: "unrelated", Value: []byte("y")},
		wire.Clear{Key: "mod.block.a"}); err != nil {
		t.Fatal(err)
	}
	// Write runs on the actor synchronously via do() — events are in.
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("events: %+v", got)
	}
	if got[1].key != "mod.block.b" || got[1].val != "2" || got[1].cleared {
		t.Fatalf("set event: %+v", got[1])
	}
	if got[2].key != "mod.block.a" || !got[2].cleared {
		t.Fatalf("clear event: %+v", got[2])
	}
}
