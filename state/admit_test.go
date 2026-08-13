package state

import (
	"errors"
	"testing"

	"github.com/panaudia/lasa/wire"
)

// admitReq builds an AdmitRequest shaped like the base profile's
// admission writes (claim group first, one group per entity, marker
// valued with the owner) without importing the profile — the engine is
// profile-blind and so are these tests.
func admitReq(clientID string, ticketed bool, capacity int, entityIDs ...string) AdmitRequest {
	claim := "lasa.client." + clientID
	req := AdmitRequest{
		ClientID: clientID,
		Rules:    allAccess,
		Ticketed: ticketed,
		Capacity: capacity,
		ClaimKey: claim,
		Groups: []wire.StateMessage{wire.Group{Ops: []wire.StateMessage{
			wire.Set{Key: claim, Value: []byte("true")},
		}}},
	}
	for _, id := range entityIDs {
		k := "lasa.entity." + id
		req.EntityKeys = append(req.EntityKeys, k)
		req.Groups = append(req.Groups, wire.Group{Ops: []wire.StateMessage{
			wire.Set{Key: k, Value: []byte(`"` + clientID + `"`)},
			wire.Set{Key: k + ".name", Value: []byte(`"x"`)},
		}})
	}
	return req
}

func mustAdmit(t *testing.T, e *Engine, req AdmitRequest) (*Conn, []string) {
	t.Helper()
	c, superseded, err := e.Admit(req)
	if err != nil {
		t.Fatalf("Admit(%s): %v", req.ClientID, err)
	}
	return c, superseded
}

func closed(c *Conn) bool {
	select {
	case <-c.Done():
		return true
	default:
		return false
	}
}

func TestAdmitFresh(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c, superseded := mustAdmit(t, e, admitReq("c1", true, 0, "e1"))
	if len(superseded) != 0 {
		t.Fatalf("superseded = %v", superseded)
	}
	if _, ok := storeGet(e, "lasa.client.c1"); !ok {
		t.Fatal("claim key missing")
	}
	if v, ok := storeGet(e, "lasa.entity.e1"); !ok || string(v) != `"c1"` {
		t.Fatalf("marker = %q, %v", v, ok)
	}
	if closed(c) {
		t.Fatal("fresh conn must be live")
	}
}

// Same client id: the newest exercise wins — predecessor closed and
// swept, successor's claim set live (core §4.4).
func TestAdmitSupersedesSameClient(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c1, _ := mustAdmit(t, e, admitReq("c1", true, 0, "e1"))
	c2, superseded := mustAdmit(t, e, admitReq("c1", true, 0, "e2"))

	if len(superseded) != 1 || superseded[0] != "c1" {
		t.Fatalf("superseded = %v", superseded)
	}
	if !closed(c1) || closed(c2) {
		t.Fatal("predecessor must be done, successor live")
	}
	if _, ok := storeGet(e, "lasa.entity.e1"); ok {
		t.Fatal("predecessor's entity must be swept")
	}
	if _, ok := storeGet(e, "lasa.entity.e2"); !ok {
		t.Fatal("successor's entity missing")
	}
	if _, ok := storeGet(e, "lasa.client.c1"); !ok {
		t.Fatal("claim key must be live (rewritten by the successor)")
	}
	// L1 for the superseded generation: its writes silently no-op.
	if err := c1.Write(wire.Set{Key: "x", Value: []byte("1")}); err != nil {
		t.Fatal(err)
	}
	if _, ok := storeGet(e, "x"); ok {
		t.Fatal("superseded generation must not write")
	}
}

// Zero-entity observers collide on the claim key alone — the previous
// blind spot (id-capabilities.md).
func TestAdmitObserverCollision(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c1, _ := mustAdmit(t, e, admitReq("c1", false, 0))
	_, superseded := mustAdmit(t, e, admitReq("c1", false, 0))
	if len(superseded) != 1 || superseded[0] != "c1" {
		t.Fatalf("superseded = %v", superseded)
	}
	if !closed(c1) {
		t.Fatal("superseded observer must be done")
	}
	if _, ok := storeGet(e, "lasa.client.c1"); !ok {
		t.Fatal("claim key must be live")
	}
}

// Ticketed cross-owner collision refuses, side-effect-free (core §4.4:
// a bystander is never severed on entity grounds).
func TestAdmitTicketedRefusesCrossOwner(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c1, _ := mustAdmit(t, e, admitReq("c1", true, 0, "e1"))
	head := e.Head()

	_, _, err := e.Admit(admitReq("c2", true, 0, "e1"))
	var owned *EntityOwnedError
	if !errors.As(err, &owned) || owned.Key != "lasa.entity.e1" {
		t.Fatalf("err = %v", err)
	}
	if closed(c1) {
		t.Fatal("incumbent must be untouched")
	}
	if _, ok := storeGet(e, "lasa.client.c2"); ok {
		t.Fatal("refused admission must write nothing")
	}
	if e.Head() != head {
		t.Fatal("refused admission must not advance the seq")
	}
}

// Unticketed newest-wins is total: a cross-owner collision supersedes
// the owning connection wholesale (dev mode, core §4.4).
func TestAdmitUnticketedSupersedesCrossOwner(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c1, _ := mustAdmit(t, e, admitReq("c1", false, 0, "e1", "e2"))
	_, superseded := mustAdmit(t, e, admitReq("c2", false, 0, "e1"))

	if len(superseded) != 1 || superseded[0] != "c1" {
		t.Fatalf("superseded = %v", superseded)
	}
	if !closed(c1) {
		t.Fatal("owner must be superseded")
	}
	// The owner's whole claim set goes, not just the colliding entity.
	if _, ok := storeGet(e, "lasa.client.c1"); ok {
		t.Fatal("owner's claim key must be swept")
	}
	if _, ok := storeGet(e, "lasa.entity.e2"); ok {
		t.Fatal("owner's other entity must be swept")
	}
	if v, ok := storeGet(e, "lasa.entity.e1"); !ok || string(v) != `"c2"` {
		t.Fatalf("entity must be re-owned: %q, %v", v, ok)
	}
}

// Capacity refuses side-effect-free, frees on Close (bare Close sweeps
// the recorded claim set), and nets out what supersession would free.
func TestAdmitCapacity(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	c1, _ := mustAdmit(t, e, admitReq("c1", true, 2, "e1", "e2"))

	if _, _, err := e.Admit(admitReq("c2", true, 2, "e3")); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := storeGet(e, "lasa.client.c2"); ok {
		t.Fatal("refused admission must write nothing")
	}

	// Same client re-admitting: its own entities count as freed.
	c1b, superseded := mustAdmit(t, e, admitReq("c1", true, 2, "e3", "e4"))
	if len(superseded) != 1 || superseded[0] != "c1" {
		t.Fatalf("superseded = %v", superseded)
	}
	_ = c1
	// Bare Close sweeps the recorded claim set and frees capacity.
	if err := c1b.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := storeGet(e, "lasa.client.c1"); ok {
		t.Fatal("bare Close must sweep the recorded claim set")
	}
	if _, _, err := e.Admit(admitReq("c2", true, 2, "e3")); err != nil {
		t.Fatalf("post-close admission: %v", err)
	}
}

// Concurrent colliding admissions serialize on the actor: exactly one
// exercise survives, every other is superseded, and the supersession
// count balances.
func TestAdmitConcurrentCollisions(t *testing.T) {
	e, _ := newTestEngine(t, nil)
	const n = 16
	conns := make([]*Conn, n)
	supersededTotal := make(chan int, n)
	donech := make(chan int, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			c, superseded, err := e.Admit(admitReq("c1", true, 0, "e1"))
			if err != nil {
				t.Errorf("Admit #%d: %v", i, err)
			}
			conns[i] = c
			supersededTotal <- len(superseded)
			donech <- i
		}(i)
	}
	total := 0
	for i := 0; i < n; i++ {
		<-donech
		total += <-supersededTotal
	}
	live := 0
	for _, c := range conns {
		if c != nil && !closed(c) {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("live conns = %d, want exactly 1", live)
	}
	if total != n-1 {
		t.Fatalf("supersessions = %d, want %d", total, n-1)
	}
	if v, ok := storeGet(e, "lasa.entity.e1"); !ok || string(v) != `"c1"` {
		t.Fatalf("marker = %q, %v", v, ok)
	}
}
