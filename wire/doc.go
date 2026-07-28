// Package wire implements every LASA byte layout: the audio packet formats
// (lasa-core.md §5), the state messages and subscribe parameter (§6.7), the
// presence packets (§7), and the wire constants (§9).
//
// Byte order: QUIC variable-length integers are big-endian by definition
// (RFC 9000 §16); all fixed-width multi-byte fields (u16, u64, f32, i16)
// are little-endian, carrying forward the rule pinned in the v1 spec
// (lasa.md §2.1).
//
// Decoders distinguish two failure modes: ErrUnknownFlags means the packet
// carries flag bits this format does not define — per the spec's discard
// rule the packet is not this format and the receiver drops it silently;
// ErrMalformed means the packet claims this format but its body is
// inconsistent with it.
package wire
