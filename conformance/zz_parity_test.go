package conformance

import (
	"sort"
	"strings"
	"testing"
)

// TestTransportParity runs last (file order) and covers C-4.1-5: every
// row that passed on a raw-QUIC fixture passed on its WebTransport twin
// and the other way round. Only the reference run has both.
func TestTransportParity(t *testing.T) {
	fixtures(t)
	report.mu.Lock()
	byFixture := map[string]map[string]Outcome{}
	for name, m := range report.byFixture {
		cp := make(map[string]Outcome, len(m))
		for id, o := range m {
			cp[id] = o
		}
		byFixture[name] = cp
	}
	report.mu.Unlock()
	pairs := map[string][2]map[string]Outcome{}
	for name, m := range byFixture {
		var mode string
		var side int
		switch {
		case strings.HasPrefix(name, "quic-"):
			mode, side = strings.TrimPrefix(name, "quic-"), 0
		case strings.HasPrefix(name, "webtransport-"):
			mode, side = strings.TrimPrefix(name, "webtransport-"), 1
		default:
			continue
		}
		p := pairs[mode]
		p[side] = m
		pairs[mode] = p
	}
	if len(pairs) == 0 {
		report.record("C-4.1-5", OutcomeNotExercised, "one transport only")
		return
	}
	var diff []string
	compared := 0
	for mode, p := range pairs {
		if p[0] == nil || p[1] == nil {
			continue
		}
		for id, o := range p[0] {
			if o2, ok := p[1][id]; ok {
				compared++
				if o != o2 {
					diff = append(diff, mode+" "+id+": quic "+string(o)+", webtransport "+string(o2))
				}
			}
		}
	}
	sort.Strings(diff)
	if len(diff) > 0 {
		report.record("C-4.1-5", OutcomeFail, strings.Join(diff, "; "))
		t.Errorf("C-4.1-5: transports disagree: %v", diff)
		return
	}
	report.record("C-4.1-5", OutcomePass, "identical outcomes on both transports")
	t.Logf("C-4.1-5: %d row outcomes identical across transports", compared)
}
