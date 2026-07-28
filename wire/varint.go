package wire

import "errors"

// QUIC variable-length integers (RFC 9000 §16): a 2-bit length prefix on
// the first byte, value big-endian in the remaining bits.

// MaxVarint is the largest value representable as a QUIC varint.
const MaxVarint = 1<<62 - 1

var errVarintRange = errors.New("wire: value exceeds varint range")
var errVarintTruncated = errors.New("wire: truncated varint")

// AppendVarint appends v to buf in the minimal QUIC varint encoding.
// v must be at most MaxVarint.
func AppendVarint(buf []byte, v uint64) []byte {
	switch {
	case v < 1<<6:
		return append(buf, byte(v))
	case v < 1<<14:
		return append(buf, 0x40|byte(v>>8), byte(v))
	case v < 1<<30:
		return append(buf, 0x80|byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	case v <= MaxVarint:
		return append(buf,
			0xC0|byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
			byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	default:
		panic(errVarintRange)
	}
}

// ParseVarint reads a QUIC varint from the front of data, returning the
// value and the number of bytes consumed. Non-minimal encodings are
// accepted, per RFC 9000; use VarintLen to check minimality where a
// canonical encoding is required.
func ParseVarint(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, errVarintTruncated
	}
	n := 1 << (data[0] >> 6)
	if len(data) < n {
		return 0, 0, errVarintTruncated
	}
	v := uint64(data[0] & 0x3F)
	for i := 1; i < n; i++ {
		v = v<<8 | uint64(data[i])
	}
	return v, n, nil
}

// VarintLen returns the length in bytes of the minimal varint encoding of v.
func VarintLen(v uint64) int {
	switch {
	case v < 1<<6:
		return 1
	case v < 1<<14:
		return 2
	case v < 1<<30:
		return 4
	default:
		return 8
	}
}
