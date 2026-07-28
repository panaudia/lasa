package wire

import (
	"fmt"

	"github.com/panaudia/lasa/ident"
)

// StateDirection selects the wire layout of state messages: ops carry a
// server-assigned seq downstream (space → client) and none upstream. The
// same type bytes parse differently per direction, so implementations MUST
// select the layout by track direction (lasa-core.md §6.7).
type StateDirection uint8

const (
	Upstream   StateDirection = iota // client → space, no seqs
	Downstream                       // space → client, seqs present
)

// StateMessage is one state-track message: Set, Clear, Group, Frontier or
// Begin. One message per MoQ object.
type StateMessage interface {
	stateMessage()
}

// Set assigns Value to Key. Seq is meaningful downstream only.
type Set struct {
	Key   string
	Value []byte
	Seq   uint64
}

// Clear removes Key. Seq is meaningful downstream only.
type Clear struct {
	Key string
	Seq uint64
}

// Group is a hint that its operations should be applied together; it
// provides no consistency guarantee and groups do not nest. Ops may
// contain only Set and Clear.
type Group struct {
	Ops []StateMessage
}

// Frontier is the promise "you will receive nothing further at or below
// Seq on this path". Downstream only.
type Frontier struct {
	Epoch [EpochSize]byte
	Seq   uint64
}

// Begin opens a catch-up payload, closed by a Frontier. Downstream only.
type Begin struct {
	Kind BeginKind
}

// BeginKind distinguishes the two catch-up payloads (lasa-core.md §6.5).
type BeginKind uint8

const (
	BeginDelta    BeginKind = 1
	BeginSnapshot BeginKind = 2
)

func (Set) stateMessage()      {}
func (Clear) stateMessage()    {}
func (Group) stateMessage()    {}
func (Frontier) stateMessage() {}
func (Begin) stateMessage()    {}

const (
	msgSet      = 1
	msgClear    = 2
	msgGroup    = 3
	msgFrontier = 4
	msgBegin    = 5
)

// AppendStateMessage appends the encoding of m for the given direction.
func AppendStateMessage(buf []byte, m StateMessage, dir StateDirection) ([]byte, error) {
	switch v := m.(type) {
	case Set:
		if !ident.ValidKey(v.Key) {
			return nil, fmt.Errorf("%w: invalid key %q", ErrMalformed, v.Key)
		}
		if len(v.Value) > MaxValueLen {
			return nil, fmt.Errorf("%w: value length %d exceeds %d", ErrMalformed, len(v.Value), MaxValueLen)
		}
		buf = append(buf, msgSet)
		buf = AppendVarint(buf, uint64(len(v.Key)))
		buf = append(buf, v.Key...)
		buf = AppendVarint(buf, uint64(len(v.Value)))
		buf = append(buf, v.Value...)
		if dir == Downstream {
			buf = AppendVarint(buf, v.Seq)
		}
		return buf, nil
	case Clear:
		if !ident.ValidKey(v.Key) {
			return nil, fmt.Errorf("%w: invalid key %q", ErrMalformed, v.Key)
		}
		buf = append(buf, msgClear)
		buf = AppendVarint(buf, uint64(len(v.Key)))
		buf = append(buf, v.Key...)
		if dir == Downstream {
			buf = AppendVarint(buf, v.Seq)
		}
		return buf, nil
	case Group:
		buf = append(buf, msgGroup)
		buf = AppendVarint(buf, uint64(len(v.Ops)))
		for _, op := range v.Ops {
			switch op.(type) {
			case Set, Clear:
			default:
				return nil, fmt.Errorf("%w: groups may contain only set and clear ops", ErrMalformed)
			}
			var err error
			buf, err = AppendStateMessage(buf, op, dir)
			if err != nil {
				return nil, err
			}
		}
		return buf, nil
	case Frontier:
		if dir != Downstream {
			return nil, fmt.Errorf("%w: frontier is downstream-only", ErrMalformed)
		}
		buf = append(buf, msgFrontier)
		buf = append(buf, v.Epoch[:]...)
		return AppendVarint(buf, v.Seq), nil
	case Begin:
		if dir != Downstream {
			return nil, fmt.Errorf("%w: begin is downstream-only", ErrMalformed)
		}
		if v.Kind != BeginDelta && v.Kind != BeginSnapshot {
			return nil, fmt.Errorf("%w: unknown begin kind %d", ErrMalformed, v.Kind)
		}
		return append(buf, msgBegin, byte(v.Kind)), nil
	default:
		return nil, fmt.Errorf("%w: unknown state message type %T", ErrMalformed, m)
	}
}

// ParseStateMessage decodes exactly one state message for the given
// direction; trailing bytes are an error (one message per MoQ object).
func ParseStateMessage(data []byte, dir StateDirection) (StateMessage, error) {
	m, n, err := parseStateMessage(data, dir, true)
	if err != nil {
		return nil, err
	}
	if n != len(data) {
		return nil, fmt.Errorf("%w: %d trailing bytes after state message", ErrMalformed, len(data)-n)
	}
	return m, nil
}

func parseStateMessage(data []byte, dir StateDirection, allowGroup bool) (StateMessage, int, error) {
	if len(data) == 0 {
		return nil, 0, fmt.Errorf("%w: empty state message", ErrMalformed)
	}
	body := data[1:]
	switch data[0] {
	case msgSet:
		key, n, err := parseKey(body)
		if err != nil {
			return nil, 0, err
		}
		vlen, vn, err := ParseVarint(body[n:])
		if err != nil {
			return nil, 0, fmt.Errorf("%w: value length: %v", ErrMalformed, err)
		}
		n += vn
		if vlen > MaxValueLen {
			return nil, 0, fmt.Errorf("%w: value length %d exceeds %d", ErrMalformed, vlen, MaxValueLen)
		}
		if uint64(len(body[n:])) < vlen {
			return nil, 0, fmt.Errorf("%w: truncated value", ErrMalformed)
		}
		m := Set{Key: key, Value: make([]byte, vlen)}
		copy(m.Value, body[n:])
		n += int(vlen)
		if dir == Downstream {
			seq, sn, err := ParseVarint(body[n:])
			if err != nil {
				return nil, 0, fmt.Errorf("%w: seq: %v", ErrMalformed, err)
			}
			m.Seq = seq
			n += sn
		}
		return m, 1 + n, nil
	case msgClear:
		key, n, err := parseKey(body)
		if err != nil {
			return nil, 0, err
		}
		m := Clear{Key: key}
		if dir == Downstream {
			seq, sn, err := ParseVarint(body[n:])
			if err != nil {
				return nil, 0, fmt.Errorf("%w: seq: %v", ErrMalformed, err)
			}
			m.Seq = seq
			n += sn
		}
		return m, 1 + n, nil
	case msgGroup:
		if !allowGroup {
			return nil, 0, fmt.Errorf("%w: groups do not nest", ErrMalformed)
		}
		count, n, err := ParseVarint(body)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: group count: %v", ErrMalformed, err)
		}
		g := Group{}
		for i := uint64(0); i < count; i++ {
			op, on, err := parseStateMessage(body[n:], dir, false)
			if err != nil {
				return nil, 0, err
			}
			switch op.(type) {
			case Set, Clear:
			default:
				return nil, 0, fmt.Errorf("%w: groups may contain only set and clear ops", ErrMalformed)
			}
			g.Ops = append(g.Ops, op)
			n += on
		}
		return g, 1 + n, nil
	case msgFrontier:
		if dir != Downstream {
			return nil, 0, fmt.Errorf("%w: frontier is downstream-only", ErrMalformed)
		}
		if len(body) < EpochSize {
			return nil, 0, fmt.Errorf("%w: truncated epoch", ErrMalformed)
		}
		var f Frontier
		copy(f.Epoch[:], body)
		seq, sn, err := ParseVarint(body[EpochSize:])
		if err != nil {
			return nil, 0, fmt.Errorf("%w: seq: %v", ErrMalformed, err)
		}
		f.Seq = seq
		return f, 1 + EpochSize + sn, nil
	case msgBegin:
		if dir != Downstream {
			return nil, 0, fmt.Errorf("%w: begin is downstream-only", ErrMalformed)
		}
		if len(body) < 1 {
			return nil, 0, fmt.Errorf("%w: truncated begin", ErrMalformed)
		}
		kind := BeginKind(body[0])
		if kind != BeginDelta && kind != BeginSnapshot {
			return nil, 0, fmt.Errorf("%w: unknown begin kind %d", ErrMalformed, kind)
		}
		return Begin{Kind: kind}, 2, nil
	default:
		return nil, 0, fmt.Errorf("%w: unknown state message type %d", ErrMalformed, data[0])
	}
}

func parseKey(data []byte) (string, int, error) {
	klen, n, err := ParseVarint(data)
	if err != nil {
		return "", 0, fmt.Errorf("%w: key length: %v", ErrMalformed, err)
	}
	if klen > ident.MaxKeyLen {
		return "", 0, fmt.Errorf("%w: key length %d exceeds %d", ErrMalformed, klen, ident.MaxKeyLen)
	}
	if uint64(len(data[n:])) < klen {
		return "", 0, fmt.Errorf("%w: truncated key", ErrMalformed)
	}
	key := string(data[n : n+int(klen)])
	if !ident.ValidKey(key) {
		return "", 0, fmt.Errorf("%w: invalid key %q", ErrMalformed, key)
	}
	return key, n + int(klen), nil
}
