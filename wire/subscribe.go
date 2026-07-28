package wire

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/panaudia/lasa/ident"
)

// Cursor is the client's resume position (lasa-core.md §6.6): persisted
// atomically, never ahead of the persisted store.
type Cursor struct {
	Epoch   [EpochSize]byte
	Seq     uint64
	SetHash [SetHashSize]byte
}

// SubscribeParams is the state subscribe message riding the SUBSCRIBE
// parameter SubscribeParamState (lasa-core.md §6.7): the subscription's
// prefix set, and the cursor when resuming. A fresh client omits the
// cursor.
type SubscribeParams struct {
	Prefixes []string
	Cursor   *Cursor
}

const subscribeFlagCursor = 1 << 0

// CanonicalPrefixes returns the canonical form of a prefix set:
// deduplicated and sorted bytewise.
func CanonicalPrefixes(prefixes []string) []string {
	out := append([]string(nil), prefixes...)
	sort.Strings(out)
	dst := out[:0]
	for i, p := range out {
		if i == 0 || p != out[i-1] {
			dst = append(dst, p)
		}
	}
	return dst
}

// SetHash computes the cursor set-hash: SHA-256 over exactly the
// concatenated [len varint][prefix bytes] entries of the canonical
// encoding (lasa-core.md §6.6, §6.7). The input must already be canonical
// (use CanonicalPrefixes).
func SetHash(canonical []string) [SetHashSize]byte {
	h := sha256.New()
	var buf []byte
	for _, p := range canonical {
		buf = AppendVarint(buf[:0], uint64(len(p)))
		h.Write(buf)
		h.Write([]byte(p))
	}
	var out [SetHashSize]byte
	h.Sum(out[:0])
	return out
}

// AppendSubscribeParams appends the encoding of p. The prefix set is
// canonicalized (deduplicated, sorted) before encoding; it must be
// non-empty and every prefix must be well-formed.
func AppendSubscribeParams(buf []byte, p *SubscribeParams) ([]byte, error) {
	prefixes := CanonicalPrefixes(p.Prefixes)
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("%w: empty prefix set is a protocol error", ErrMalformed)
	}
	for _, pre := range prefixes {
		if !ident.ValidKeyPrefix(pre) {
			return nil, fmt.Errorf("%w: invalid prefix %q", ErrMalformed, pre)
		}
	}
	var flags byte
	if p.Cursor != nil {
		flags |= subscribeFlagCursor
	}
	buf = append(buf, flags)
	buf = AppendVarint(buf, uint64(len(prefixes)))
	for _, pre := range prefixes {
		buf = AppendVarint(buf, uint64(len(pre)))
		buf = append(buf, pre...)
	}
	if p.Cursor != nil {
		buf = append(buf, p.Cursor.Epoch[:]...)
		buf = AppendVarint(buf, p.Cursor.Seq)
		buf = append(buf, p.Cursor.SetHash[:]...)
	}
	return buf, nil
}

// ParseSubscribeParams decodes a state subscribe message. The encoding is
// required to be canonical: minimal varints, prefixes deduplicated and
// sorted bytewise, no unknown flag bits, no trailing bytes. Anything else
// is rejected — the set-hash preimage must be unambiguous on both sides.
func ParseSubscribeParams(data []byte) (*SubscribeParams, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty subscribe params", ErrMalformed)
	}
	flags := data[0]
	if flags&^byte(subscribeFlagCursor) != 0 {
		return nil, fmt.Errorf("%w: unknown subscribe flag bits %#x", ErrMalformed, flags)
	}
	rest := data[1:]

	count, n, err := parseCanonicalVarint(rest)
	if err != nil {
		return nil, fmt.Errorf("%w: prefix count: %v", ErrMalformed, err)
	}
	rest = rest[n:]
	if count == 0 {
		return nil, fmt.Errorf("%w: empty prefix set is a protocol error", ErrMalformed)
	}

	p := &SubscribeParams{}
	for i := uint64(0); i < count; i++ {
		plen, n, err := parseCanonicalVarint(rest)
		if err != nil {
			return nil, fmt.Errorf("%w: prefix length: %v", ErrMalformed, err)
		}
		rest = rest[n:]
		if plen > ident.MaxKeyLen {
			return nil, fmt.Errorf("%w: prefix length %d exceeds %d", ErrMalformed, plen, ident.MaxKeyLen)
		}
		if uint64(len(rest)) < plen {
			return nil, fmt.Errorf("%w: truncated prefix", ErrMalformed)
		}
		pre := string(rest[:plen])
		rest = rest[plen:]
		if !ident.ValidKeyPrefix(pre) {
			return nil, fmt.Errorf("%w: invalid prefix %q", ErrMalformed, pre)
		}
		if i > 0 && pre <= p.Prefixes[len(p.Prefixes)-1] {
			return nil, fmt.Errorf("%w: prefix set not in canonical order", ErrMalformed)
		}
		p.Prefixes = append(p.Prefixes, pre)
	}

	if flags&subscribeFlagCursor != 0 {
		if len(rest) < EpochSize {
			return nil, fmt.Errorf("%w: truncated cursor epoch", ErrMalformed)
		}
		c := &Cursor{}
		copy(c.Epoch[:], rest)
		rest = rest[EpochSize:]
		seq, n, err := parseCanonicalVarint(rest)
		if err != nil {
			return nil, fmt.Errorf("%w: cursor seq: %v", ErrMalformed, err)
		}
		c.Seq = seq
		rest = rest[n:]
		if len(rest) < SetHashSize {
			return nil, fmt.Errorf("%w: truncated set-hash", ErrMalformed)
		}
		copy(c.SetHash[:], rest)
		rest = rest[SetHashSize:]
		p.Cursor = c
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("%w: %d trailing bytes", ErrMalformed, len(rest))
	}
	return p, nil
}

// parseCanonicalVarint parses a varint and rejects non-minimal encodings.
func parseCanonicalVarint(data []byte) (uint64, int, error) {
	v, n, err := ParseVarint(data)
	if err != nil {
		return 0, 0, err
	}
	if n != VarintLen(v) {
		return 0, 0, fmt.Errorf("non-minimal varint encoding")
	}
	return v, n, nil
}
