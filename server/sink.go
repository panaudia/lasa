package server

import (
	"sync"
	"sync/atomic"

	"github.com/panaudia/moqtransport"

	"github.com/panaudia/lasa/wire"
)

// sinkManager owns the render lifecycle: one render per (entity, format),
// started on the first subscription and stopped when the last subscriber
// leaves (subscription is the only signal, lasa-core.md §3.2).
type sinkManager struct {
	backend     Backend
	sampleEpoch func(sampleTS uint64) uint64
	mu          sync.Mutex
	renders     map[sinkKey]*render
	// decl is the effective sink-redundancy offset per live entity
	// (declared by the entity, filtered through Config.SinkRedundancy
	// at admission; absent = no emission). Read at render start —
	// emission policy is static per session (lasa-core.md §4.2;
	// plan/sink-redundancy.md v1).
	declMu sync.Mutex
	decl   map[string]uint8
}

type sinkKey struct{ entity, format string }

type render struct {
	key     sinkKey
	writer  *sinkWriter
	session SinkSession
	// pending counts subscribers between ensure and
	// addPublisher/release (guarded by sinkManager.mu): a render with
	// pending holds is never stopped, and a failed Accept releases its
	// hold instead of leaking a zero-subscriber render (§3.2: the
	// server renders only while a subscription is active).
	pending int
}

type pubEntry struct {
	pub   moqtransport.Publisher
	owner *connSession
}

func newSinkManager(b Backend, sampleEpoch func(uint64) uint64) *sinkManager {
	return &sinkManager{
		backend: b, sampleEpoch: sampleEpoch,
		renders: map[sinkKey]*render{}, decl: map[string]uint8{},
	}
}

// setDecl / clearDecl bracket an entity's live effective sink-redundancy
// offset (entity join / departure).
func (m *sinkManager) setDecl(entityID string, offset uint8) {
	m.declMu.Lock()
	m.decl[entityID] = offset
	m.declMu.Unlock()
}

func (m *sinkManager) clearDecl(entityID string) {
	m.declMu.Lock()
	delete(m.decl, entityID)
	m.declMu.Unlock()
}

func (m *sinkManager) declOf(entityID string) uint8 {
	m.declMu.Lock()
	defer m.declMu.Unlock()
	return m.decl[entityID]
}

// ensure returns the (entity, format) render, starting it if absent,
// and takes a pending hold on it: the caller MUST follow with
// addPublisher (subscribe completed) or release (it did not).
func (m *sinkManager) ensure(entityID, format string) (*render, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := sinkKey{entityID, format}
	if r, ok := m.renders[key]; ok {
		r.pending++
		return r, nil
	}
	r := &render{key: key, writer: &sinkWriter{sampleEpoch: m.sampleEpoch, red: m.declOf(entityID)}}
	r.writer.mgr, r.writer.r = m, r
	session, err := m.backend.StartSink(entityID, format, r.writer)
	if err != nil {
		return nil, err
	}
	r.session = session
	r.pending = 1
	m.renders[key] = r
	return r, nil
}

func (m *sinkManager) addPublisher(r *render, pub moqtransport.Publisher, owner *connSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.pending--
	old := r.writer.pubs.Load()
	var next []pubEntry
	if old != nil {
		next = append(next, *old...)
	}
	next = append(next, pubEntry{pub: pub, owner: owner})
	r.writer.pubs.Store(&next)
}

// release undoes ensure's hold when the subscribe never completed
// (Accept failure): the render stops unless a publisher or another
// pending subscriber still holds it.
func (m *sinkManager) release(r *render) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.pending--
	pubs := r.writer.pubs.Load()
	if r.pending == 0 && (pubs == nil || len(*pubs) == 0) && m.renders[r.key] == r {
		r.session.Stop()
		delete(m.renders, r.key)
	}
}

// dropConn removes every publisher owned by cs, stopping renders whose
// last subscriber left.
func (m *sinkManager) dropConn(cs *connSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, r := range m.renders {
		old := r.writer.pubs.Load()
		if old == nil {
			continue
		}
		next := make([]pubEntry, 0, len(*old))
		for _, pe := range *old {
			if pe.owner != cs {
				next = append(next, pe)
			}
		}
		if len(next) == len(*old) {
			continue
		}
		r.writer.pubs.Store(&next)
		if len(next) == 0 && r.pending == 0 {
			r.session.Stop()
			delete(m.renders, key)
		}
	}
}

// dropEntity stops every render OF the given entity and ends its
// subscribers' tracks with SUBSCRIBE_DONE (track ended): entity
// departure terminates the entity's sink tracks, and a subscriber who
// wants the entity's return re-subscribes — subscription stays the only
// lifecycle signal (decided 2026-08-01, option A; the alternative — a
// render entry surviving departure — would hand a reused stale render
// to the entity's successor). Called from teardown AFTER EntityLeft:
// once the embedder's record is gone a racing subscribe cannot start a
// fresh render for the departed entity, and one attached to the old
// entry before our delete still gets its SUBSCRIBE_DONE here.
func (m *sinkManager) dropEntity(entityID string) {
	m.mu.Lock()
	var stopped []*render
	for key, r := range m.renders {
		if key.entity == entityID {
			stopped = append(stopped, r)
			delete(m.renders, key)
		}
	}
	m.mu.Unlock()
	for _, r := range stopped {
		r.session.Stop()
		if pubs := r.writer.pubs.Load(); pubs != nil {
			for _, pe := range *pubs {
				_ = pe.pub.CloseWithError(uint64(moqtransport.ErrorCodeSubscribeDoneTrackEnded), "entity left")
			}
		}
	}
}

// drop removes one failed publisher (send error = unsubscribed/closed).
func (m *sinkManager) drop(r *render, failed moqtransport.Publisher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := r.writer.pubs.Load()
	if old == nil {
		return
	}
	next := make([]pubEntry, 0, len(*old))
	for _, pe := range *old {
		if pe.pub != failed {
			next = append(next, pe)
		}
	}
	r.writer.pubs.Store(&next)
	if len(next) == 0 {
		if m.renders[r.key] == r {
			r.session.Stop()
			delete(m.renders, r.key)
		}
	}
}

// sinkWriter is the push surface handed to the embedder: the hot-path
// egress. WriteFrame frames into a scratch buffer reused across frames
// (the one framing copy; amortized zero allocation) and sends
// synchronously to every subscriber. Driven by one render goroutine;
// not safe for concurrent use.
type sinkWriter struct {
	mgr *sinkManager
	r   *render

	seq        uint64
	scratch    []byte
	lastAnchor uint64
	sentFirst  bool
	// Sink-track repeat emitter (lasa-core.md §5.1; the entity's §4.2
	// declaration is the request AND the bound). red is the effective
	// offset (0 = off, static per session); history holds the previous
	// frames' payloads — 8 slots so slot (seq−7)%8 is never the slot
	// being written at seq. Slices are reused across laps (amortized
	// zero allocation, same as scratch).
	red     uint8
	history [8][]byte
	// sampleEpoch maps the embedder's sample clock to unix-epoch µs for
	// the informational anchors — the shell's only contact with epoch
	// time (design §1: one clock; the value derives from the render
	// tick, not from per-anchor wall reads). nil disables anchors.
	sampleEpoch func(sampleTS uint64) uint64
	pubs        atomic.Pointer[[]pubEntry]
	failed      []moqtransport.Publisher
}

// tsCadenceSamples is the on-wire anchor cadence in the sample domain:
// on stream start and on the order of once per second (lasa-core.md
// §5.1) — 48 000 samples at the 48 kHz invariant.
const tsCadenceSamples = 48_000

func (w *sinkWriter) WriteFrame(frame []byte, sampleTS uint64) error {
	seq := w.seq
	w.seq++ // sink tracks are continuous: ids advance regardless
	if w.red > 0 {
		// The history ring advances with the id space even while no
		// subscriber is attached, so a mid-render join sees correctly
		// referenced repeats immediately.
		slot := seq % uint64(len(w.history))
		w.history[slot] = append(w.history[slot][:0], frame...)
	}
	pubs := w.pubs.Load()
	if pubs == nil || len(*pubs) == 0 {
		return nil
	}
	pkt := wire.SinkPacket{Audio: frame}
	if w.red > 0 && seq >= uint64(w.red) {
		// The referenced packet is always audio-bearing (§5.1 MUST):
		// sink tracks are continuous at the frame cadence.
		dup := w.history[(seq-uint64(w.red))%uint64(len(w.history))]
		if len(dup) > 0 {
			pkt.Redundancy = &wire.Redundancy{Offset: w.red, Audio: dup}
		}
	}
	if w.sampleEpoch != nil && (!w.sentFirst || sampleTS-w.lastAnchor >= tsCadenceSamples) {
		ts := w.sampleEpoch(sampleTS)
		pkt.Timestamp = &ts
		w.lastAnchor = sampleTS
		w.sentFirst = true
	}
	var err error
	if w.scratch, err = wire.AppendSink(w.scratch[:0], &pkt); err != nil {
		return err
	}
	w.failed = w.failed[:0]
	for _, pe := range *pubs {
		if e := pe.pub.SendDatagram(moqtransport.Object{
			GroupID:              seq,
			ObjectID:             0,
			ForwardingPreference: moqtransport.ObjectForwardingPreferenceDatagram,
			Payload:              w.scratch,
		}); e != nil {
			w.failed = append(w.failed, pe.pub)
		}
	}
	for _, f := range w.failed {
		w.mgr.drop(w.r, f)
	}
	return nil
}
