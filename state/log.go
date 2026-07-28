package state

import "sync"

// logEntry is one op in the broadcast log. Value bytes are shared with the
// store map (immutable by convention). Group boundaries are recorded so
// live delivery can honour the preserve-groups SHOULD.
type logEntry struct {
	seq        uint64
	key        string
	value      []byte
	tombstone  bool
	groupFirst bool
	groupLast  bool
}

// broadcastLog is the bounded in-memory fan-out window
// (state-engine-design.md §1). The single actor appends; subscriber
// readers consume at their own pace via cursors. Bounded in entries AND
// bytes; a reader whose cursor falls off the tail is lagged. It is a
// broadcast log, not a WAL: it plays no durability role.
type broadcastLog struct {
	mu         sync.Mutex
	entries    []logEntry
	firstSeq   uint64 // seq of entries[0]; meaningful when len > 0
	headSeq    uint64 // highest appended seq; 0 before any append
	bytes      int
	maxEntries int
	maxBytes   int
	notify     chan struct{}
}

func newBroadcastLog(maxEntries, maxBytes int) *broadcastLog {
	return &broadcastLog{
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		notify:     make(chan struct{}),
	}
}

// append adds entries (one op or one whole group — a group is appended
// atomically, invariant F6) and wakes all waiting readers.
func (l *broadcastLog) append(es ...logEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range es {
		if len(l.entries) == 0 {
			l.firstSeq = e.seq
		}
		l.entries = append(l.entries, e)
		l.headSeq = e.seq
		l.bytes += len(e.value) + len(e.key)
	}
	for (len(l.entries) > l.maxEntries || l.bytes > l.maxBytes) && len(l.entries) > 0 {
		drop := l.entries[0]
		l.bytes -= len(drop.value) + len(drop.key)
		l.entries = l.entries[1:]
		l.firstSeq = drop.seq + 1
	}
	l.wakeLocked()
}

// read returns all entries with seq >= from. lagged reports that entries
// the reader had not consumed were evicted (or that from lands inside a
// partially evicted group).
func (l *broadcastLog) read(from uint64) (es []logEntry, lagged bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) == 0 {
		if from <= l.headSeq {
			return nil, true // everything the reader wanted is gone
		}
		return nil, false
	}
	if from < l.firstSeq {
		return nil, true
	}
	i := int(from - l.firstSeq)
	if i >= len(l.entries) {
		return nil, false
	}
	// A single op carries both group flags; a cursor landing on an entry
	// that does not open a group means the group's head was evicted.
	if !l.entries[i].groupFirst {
		return nil, true
	}
	out := make([]logEntry, len(l.entries)-i)
	copy(out, l.entries[i:])
	return out, false
}

// waitCh returns a channel closed on the next append (or wake). Take it
// BEFORE a read that might come back empty.
func (l *broadcastLog) waitCh() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.notify
}

// wake wakes all waiting readers without appending (used by Tick so
// blocked readers re-check frontier deadlines and termination).
func (l *broadcastLog) wake() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.wakeLocked()
}

func (l *broadcastLog) wakeLocked() {
	close(l.notify)
	l.notify = make(chan struct{})
}
