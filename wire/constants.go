package wire

// Wire constants (lasa-core.md §9).
const (
	// SetupParamConnectionConfig is the CLIENT_SETUP parameter key carrying
	// the Connection Config JSON (odd: length-prefixed bytes).
	SetupParamConnectionConfig uint64 = 0x1A5A1

	// SubscribeParamState is the SUBSCRIBE parameter key carrying the state
	// subscribe message (odd: length-prefixed bytes).
	SubscribeParamState uint64 = 0x1A5A3
)

// Session error codes (lasa-core.md §4.6). Codes fit in 32 bits (the
// WebTransport bound) and sit in a LASA range distinct from MoQ's own
// session-terminate codes.
const (
	ErrCodeMalformedConfig   uint32 = 0x1A5A00 // malformed / schema-invalid Connection Config
	ErrCodeInvalidTicket     uint32 = 0x1A5A01 // signature, expired, not yet valid, wrong audience
	ErrCodeClientIDMismatch  uint32 = 0x1A5A02 // client_id does not match the ticket claim
	ErrCodeIDCollision       uint32 = 0x1A5A03 // unticketed live client or entity id
	ErrCodeEntityCollision   uint32 = 0x1A5A04 // entity id collides with another client's live entity
	ErrCodeAdHocNotPermitted uint32 = 0x1A5A05 // ad-hoc entities not permitted for this ticket
	ErrCodeBlocked           uint32 = 0x1A5A06 // blocked
	ErrCodeAtCapacity        uint32 = 0x1A5A07 // space at capacity
)

const (
	// MaxValueLen is the maximum state value length in bytes.
	MaxValueLen = 65536

	// EpochSize is the width of the opaque state epoch in bytes.
	EpochSize = 8

	// SetHashSize is the width of the subscribe cursor's set-hash in bytes.
	SetHashSize = 32
)

// Audio framing (lasa-core.md §1, §5): Opus at 48 kHz in 5 ms frames, one
// frame per packet.
const (
	SampleRate        = 48000
	FrameSamples      = 240 // 5 ms at 48 kHz
	FrameMicroseconds = 5000
)
