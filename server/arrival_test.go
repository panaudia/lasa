package server

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type countingHandler struct{ seqs []uint64 }

func (c *countingHandler) WritePacket(seq uint64, _ []byte) { c.seqs = append(c.seqs, seq) }

// The record a session writes has the probes' layout: the 16-byte
// header, one 32-byte record per packet in arrival order with the
// wall-clock stamp and the payload length, and a stop mark last.
func TestArrivalRecordLayout(t *testing.T) {
	dir := t.TempDir()
	rec := NewArrivalRecorder(dir)
	inner := &countingHandler{}
	a := rec.wrap("ent-1", inner)

	before := time.Now().UnixNano()
	a.WritePacket(10, make([]byte, 120))
	a.WritePacket(11, make([]byte, 121))
	a.WritePacket(13, make([]byte, 1200))
	after := time.Now().UnixNano()
	a.flush()
	a.flush() // idempotent

	if len(inner.seqs) != 3 || inner.seqs[2] != 13 {
		t.Fatalf("inner handler saw %v", inner.seqs)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "ent-1-*.rec"))
	if len(files) != 1 {
		t.Fatalf("want one record file, got %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:8]) != arrivalMagic || binary.LittleEndian.Uint32(data[8:]) != arrivalFormat {
		t.Fatalf("bad header %x", data[:16])
	}
	if want := arrivalHeaderSize + 4*arrivalRecordSize; len(data) != want {
		t.Fatalf("length %d, want %d (3 packets + stop)", len(data), want)
	}
	wantSeq := []uint64{10, 11, 13}
	wantLen := []uint32{120, 121, 1200}
	var last int64
	for i := 0; i < 3; i++ {
		p := data[arrivalHeaderSize+i*arrivalRecordSize:]
		if got := binary.LittleEndian.Uint64(p[0:]); got != wantSeq[i] {
			t.Errorf("record %d seq %d, want %d", i, got, wantSeq[i])
		}
		ts := int64(binary.LittleEndian.Uint64(p[8:]))
		if ts < before || ts > after || ts < last {
			t.Errorf("record %d stamp %d outside [%d, %d] or out of order", i, ts, before, after)
		}
		last = ts
		if got := binary.LittleEndian.Uint64(p[16:]); got != 0 {
			t.Errorf("record %d server_us %d, want 0", i, got)
		}
		if got := binary.LittleEndian.Uint32(p[24:]); got != wantLen[i] {
			t.Errorf("record %d payload_len %d, want %d", i, got, wantLen[i])
		}
		if got := binary.LittleEndian.Uint32(p[28:]); got != 0 {
			t.Errorf("record %d flags %d, want 0", i, got)
		}
	}
	stop := data[arrivalHeaderSize+3*arrivalRecordSize:]
	if binary.LittleEndian.Uint32(stop[28:]) != arrivalFlagEvent || binary.LittleEndian.Uint32(stop[24:]) != arrivalEventStop {
		t.Errorf("last record is not the stop mark: %x", stop)
	}
}

// Past MaxRecords the record ends and later packets still reach the
// handler.
func TestArrivalRecordCap(t *testing.T) {
	dir := t.TempDir()
	rec := &ArrivalRecorder{Dir: dir, MaxRecords: 5}
	inner := &countingHandler{}
	a := rec.wrap("ent-2", inner)
	for i := 0; i < 9; i++ {
		a.WritePacket(uint64(i), []byte{1})
	}
	a.flush()
	if len(inner.seqs) != 9 {
		t.Fatalf("inner handler saw %d packets, want 9", len(inner.seqs))
	}
	if a.dropped.Load() != 4 {
		t.Errorf("dropped %d, want 4", a.dropped.Load())
	}
	files, _ := filepath.Glob(filepath.Join(dir, "ent-2-*.rec"))
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := arrivalHeaderSize + 6*arrivalRecordSize; len(data) != want {
		t.Fatalf("length %d, want %d (5 packets + stop)", len(data), want)
	}
}

// A flush that cannot write reports through Err and does not panic.
func TestArrivalRecordFlushError(t *testing.T) {
	var got error
	rec := &ArrivalRecorder{Dir: filepath.Join(t.TempDir(), "file-not-dir"), Err: func(_ string, err error) { got = err }}
	if err := os.WriteFile(rec.Dir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := rec.wrap("ent-3", &countingHandler{})
	a.WritePacket(1, []byte{1})
	a.flush()
	if got == nil {
		t.Fatal("expected a flush error")
	}
}
