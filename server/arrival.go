package server

// The server-side arrival record: a test fixture for the iOS transport
// gates (lasa-planning/plan/ios-transport-tests.md §5.2, S3 (b) to (e)).
// The phone cannot observe its own packets arriving at the server, so
// when Config.ArrivalRecorder is set every entity-source datagram the
// server ingests is stamped on arrival and written, one file per entity
// session, in the 32-byte record layout the probes write
// (lasa-ios-probe/README.md), so analysis/analyse.py reads both ends.
//
// This is the one place the server reads the wall clock (the design is
// wall-clock-free; plan/panaudia-server-design.md §1), which is why it
// is a fixture behind a flag and nil in production. The record's
// arrival stamps are unix nanoseconds, the same reading as the probes',
// so one-way delay lines up between the two files.

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	arrivalRecordSize   = 32
	arrivalHeaderSize   = 16
	arrivalMagic        = "LASAPRB1"
	arrivalFormat       = 1
	arrivalFlagServerUS = 1 << 0
	arrivalFlagEvent    = 1 << 1
	arrivalEventStop    = 9 // the probes' "stop" mark, written at flush
)

// ArrivalRecorder writes the arrival record for every entity source
// the server ingests. Zero value unusable; make one with
// NewArrivalRecorder.
type ArrivalRecorder struct {
	// Dir receives one file per entity session, named
	// <entity>-<join time, RFC 3339>.rec, created if missing.
	Dir string
	// MaxRecords bounds one session's record; past it packets are
	// counted as dropped and the file ends. 200 packets per second for
	// thirty minutes is 360,000 records, 11.5 MB. Default when 0.
	MaxRecords int
	// Err receives a flush failure; nil ignores it. The server has no
	// logger of its own; the embedder passes its own.
	Err func(entityID string, err error)
}

// NewArrivalRecorder records into dir with the defaults.
func NewArrivalRecorder(dir string) *ArrivalRecorder {
	return &ArrivalRecorder{Dir: dir}
}

func (r *ArrivalRecorder) maxRecords() int {
	if r.MaxRecords > 0 {
		return r.MaxRecords
	}
	return 200 * 60 * 30
}

// wrap returns h with arrival recording in front of it, for one entity
// session starting now.
func (r *ArrivalRecorder) wrap(entityID string, h EntityHandler) *arrivalRecord {
	return &arrivalRecord{
		inner:    h,
		rec:      r,
		entityID: entityID,
		joined:   time.Now(),
		cap:      r.maxRecords(),
	}
}

// arrivalRecord is the per-session wrapper. WritePacket is the ingress
// hot path (one goroutine, the datagram receiver): it stamps, appends
// and forwards. flush runs once, at teardown, from another goroutine;
// the atomic count publishes complete records to it, the probe
// buffer's scheme.
type arrivalRecord struct {
	inner    EntityHandler
	rec      *ArrivalRecorder
	entityID string
	joined   time.Time
	cap      int

	buf     []byte // header space, then records; grows by doubling
	n       atomic.Uint64
	dropped atomic.Uint64
	flushMu sync.Mutex
	flushed bool
}

var _ EntityHandler = (*arrivalRecord)(nil)

func (a *arrivalRecord) WritePacket(seq uint64, payload []byte) {
	a.append(seq, time.Now().UnixNano(), uint32(len(payload)), 0)
	a.inner.WritePacket(seq, payload)
}

func (a *arrivalRecord) append(seq uint64, nowNS int64, payloadLen uint32, flags uint32) {
	i := int(a.n.Load())
	if i >= a.cap {
		a.dropped.Add(1)
		return
	}
	off := arrivalHeaderSize + i*arrivalRecordSize
	if need := off + arrivalRecordSize; need > len(a.buf) {
		// The datagram goroutine owns buf; growth is amortised and
		// bounded by cap. flush only reads below n, so a reslice here
		// is published by the n.Store below.
		grow := 2 * len(a.buf)
		if grow < need {
			grow = need
		}
		if limit := arrivalHeaderSize + a.cap*arrivalRecordSize; grow > limit {
			grow = limit
		}
		nb := make([]byte, grow)
		copy(nb, a.buf)
		a.buf = nb
	}
	p := a.buf[off : off+arrivalRecordSize]
	binary.LittleEndian.PutUint64(p[0:], seq)
	binary.LittleEndian.PutUint64(p[8:], uint64(nowNS))
	binary.LittleEndian.PutUint64(p[16:], 0)
	binary.LittleEndian.PutUint32(p[24:], payloadLen)
	binary.LittleEndian.PutUint32(p[28:], flags)
	a.n.Store(uint64(i + 1))
}

// path is the file this session's record lands in.
func (a *arrivalRecord) path() string {
	stamp := a.joined.UTC().Format("2006-01-02T15:04:05Z")
	return filepath.Join(a.rec.Dir, filepath.Base(a.entityID)+"-"+stamp+".rec")
}

// flush writes the record. Called once at the entity's teardown; a
// second call is a no-op. Packets that arrive during the write, after
// the count was read, are not in the file.
func (a *arrivalRecord) flush() {
	a.flushMu.Lock()
	defer a.flushMu.Unlock()
	if a.flushed {
		return
	}
	a.flushed = true
	if err := a.write(); err != nil && a.rec.Err != nil {
		a.rec.Err(a.entityID, err)
	}
}

func (a *arrivalRecord) write() error {
	n := int(a.n.Load())
	// The stop mark, after the last packet, so the file says when the
	// session ended even if the sender had gone quiet.
	end := n
	var tail [arrivalRecordSize]byte
	binary.LittleEndian.PutUint64(tail[8:], uint64(time.Now().UnixNano()))
	binary.LittleEndian.PutUint32(tail[24:], arrivalEventStop)
	binary.LittleEndian.PutUint32(tail[28:], arrivalFlagEvent)

	// Header word 3 is the Go probe's collector count; 0 here. A
	// session that overran MaxRecords is told by its length.
	var hdr [arrivalHeaderSize]byte
	copy(hdr[0:8], arrivalMagic)
	binary.LittleEndian.PutUint32(hdr[8:], arrivalFormat)
	binary.LittleEndian.PutUint32(hdr[12:], 0)

	if err := os.MkdirAll(a.rec.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(a.path(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(hdr[:]); err != nil {
		f.Close()
		return err
	}
	if end > 0 {
		if _, err := f.Write(a.buf[arrivalHeaderSize : arrivalHeaderSize+end*arrivalRecordSize]); err != nil {
			f.Close()
			return err
		}
	}
	if _, err := f.Write(tail[:]); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", a.path(), err)
	}
	return nil
}
