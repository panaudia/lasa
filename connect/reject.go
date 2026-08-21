package connect

import (
	"fmt"

	"github.com/panaudia/lasa/wire"
)

// Rejection is a connection rejection (lasa-core.md §4.6): a deliberate
// close of the session carrying a code and an advisory reason string.
// Correctness MUST ride the code alone.
type Rejection struct {
	Code   uint32
	Reason string
}

func (r *Rejection) Error() string {
	return fmt.Sprintf("rejected (0x%x): %s", r.Code, r.Reason)
}

func rejectf(code uint32, format string, args ...any) *Rejection {
	return &Rejection{Code: code, Reason: fmt.Sprintf(format, args...)}
}

// Convenience constructors for the codes connect itself raises; the
// admission-time collision code (0x1A5A04; 0x1A5A03 is retired, never
// issued) and capacity/blocked codes belong to the server.
func rejectMalformed(format string, args ...any) *Rejection {
	return rejectf(wire.ErrCodeMalformedConfig, format, args...)
}

func rejectTicket(format string, args ...any) *Rejection {
	return rejectf(wire.ErrCodeInvalidTicket, format, args...)
}
