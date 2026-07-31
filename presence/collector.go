package presence

// The Collector — the ingest half of presence as the third singleton
// (plan/panaudia-server-design.md §6; siblings: the state engine, the
// mixer). Pose updates arrive at packet rate (up to 200 Hz per entity,
// on transport receive goroutines) and land in per-entity latest-wins
// slots — the slot IS the rate reducer: no queue, no decimation logic,
// no timestamps. A cadence pacer samples the slots and fans the
// aggregated entries to entry-level subscribers (the track Author,
// recording, analytics, a future cloud presence tier).
//
// The external-service seam runs between collector and subscribers:
// the collector stays process-local beside ingress (packet-rate updates
// never cross a network); everything downstream of Tick is
// location-independent.

import (
	"sync"
	"time"

	"github.com/panaudia/lasa/wire"
)

// Subscriber receives one sampled roster per tick. entries is shared,
// read-only, and valid only during the call.
type Subscriber func(now time.Time, entries []Entry)

// Slot is one entity's latest-wins presence sample. UpdatePose is
// called from the entity's receive path (single writer); the sampler
// reads at cadence. The mutex is per-entity and the critical sections
// are a handful of word copies — no cross-entity contention, no
// blocking of consequence on the receive path.
type Slot struct {
	mu        sync.Mutex
	id        string
	headFrame bool
	pose      wire.Pose
	loudness  func() wire.Loudness
}

// UpdatePose records the latest pose (latest-wins; call after any
// receiver-side validation such as dof enforcement).
func (s *Slot) UpdatePose(p wire.Pose) {
	s.mu.Lock()
	s.pose = p
	s.mu.Unlock()
}

func (s *Slot) sample() Entry {
	s.mu.Lock()
	e := Entry{EntityID: s.id, Pose: s.pose, HeadFrame: s.headFrame}
	s.mu.Unlock()
	if s.loudness != nil {
		e.Loudness = s.loudness()
	}
	return e
}

// Collector owns the slots and the entry-level pubsub.
type Collector struct {
	mu    sync.Mutex
	slots map[string]*Slot
	subs  map[any]Subscriber

	scratch []Entry // Tick-owned
}

func NewCollector() *Collector {
	return &Collector{slots: map[string]*Slot{}, subs: map[any]Subscriber{}}
}

// Register creates (or replaces) the entity's slot. The returned Slot
// is the entity's pose-update handle — hold it; no per-update map
// lookups. loudness (may be nil) is sampled at tick time; headFrame
// marks a head-frame entity (its poses are head-relative — core §7).
// The slot's pose is zero until the first UpdatePose: seed it with the
// entity's initial pose if presence before first movement matters.
func (c *Collector) Register(id string, headFrame bool, loudness func() wire.Loudness) *Slot {
	s := &Slot{id: id, headFrame: headFrame, loudness: loudness}
	c.mu.Lock()
	c.slots[id] = s
	c.mu.Unlock()
	return s
}

// Remove drops the entity from the roster. Idempotent; the shell also
// calls this on entity departure so no embedder can forget it.
func (c *Collector) Remove(id string) {
	c.mu.Lock()
	delete(c.slots, id)
	c.mu.Unlock()
}

// Subscribe registers an entry-level subscriber under key.
func (c *Collector) Subscribe(key any, fn Subscriber) {
	c.mu.Lock()
	c.subs[key] = fn
	c.mu.Unlock()
}

// Unsubscribe removes the subscriber registered under key.
func (c *Collector) Unsubscribe(key any) {
	c.mu.Lock()
	delete(c.subs, key)
	c.mu.Unlock()
}

// Run paces Tick at interval until stop closes — a pacer in the design
// §1 sense: it schedules sampling and mints no time values into any
// data plane (the `now` it passes feeds only control-plane cadence
// decisions in subscribers).
func (c *Collector) Run(interval time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			c.Tick(now)
		}
	}
}

// Tick samples every slot once and fans the roster to every
// subscriber. One goroutine drives Tick.
func (c *Collector) Tick(now time.Time) {
	c.mu.Lock()
	c.scratch = c.scratch[:0]
	for _, s := range c.slots {
		c.scratch = append(c.scratch, s.sample())
	}
	subs := make([]Subscriber, 0, len(c.subs))
	for _, fn := range c.subs {
		subs = append(subs, fn)
	}
	c.mu.Unlock()
	for _, fn := range subs {
		fn(now, c.scratch)
	}
}
