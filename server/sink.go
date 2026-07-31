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
}

type sinkKey struct{ entity, format string }

type render struct {
	key     sinkKey
	writer  *sinkWriter
	session SinkSession
}

type pubEntry struct {
	pub   moqtransport.Publisher
	owner *connSession
}

func newSinkManager(b Backend, sampleEpoch func(uint64) uint64) *sinkManager {
	return &sinkManager{backend: b, sampleEpoch: sampleEpoch, renders: map[sinkKey]*render{}}
}

// ensure returns the (entity, format) render, starting it if absent.
func (m *sinkManager) ensure(entityID, format string) (*render, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := sinkKey{entityID, format}
	if r, ok := m.renders[key]; ok {
		return r, nil
	}
	r := &render{key: key, writer: &sinkWriter{sampleEpoch: m.sampleEpoch}}
	r.writer.mgr, r.writer.r = m, r
	session, err := m.backend.StartSink(entityID, format, r.writer)
	if err != nil {
		return nil, err
	}
	r.session = session
	m.renders[key] = r
	return r, nil
}

func (m *sinkManager) addPublisher(r *render, pub moqtransport.Publisher, owner *connSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := r.writer.pubs.Load()
	var next []pubEntry
	if old != nil {
		next = append(next, *old...)
	}
	next = append(next, pubEntry{pub: pub, owner: owner})
	r.writer.pubs.Store(&next)
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
		if len(next) == 0 {
			r.session.Stop()
			delete(m.renders, key)
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
	pubs := w.pubs.Load()
	if pubs == nil || len(*pubs) == 0 {
		w.seq++ // sink tracks are continuous: ids advance regardless
		return nil
	}
	pkt := wire.SinkPacket{Audio: frame}
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
			GroupID:              w.seq,
			ObjectID:             0,
			ForwardingPreference: moqtransport.ObjectForwardingPreferenceDatagram,
			Payload:              w.scratch,
		}); e != nil {
			w.failed = append(w.failed, pe.pub)
		}
	}
	w.seq++
	for _, f := range w.failed {
		w.mgr.drop(w.r, f)
	}
	return nil
}
