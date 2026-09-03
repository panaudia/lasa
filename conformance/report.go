package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
)

// Outcome is one of the four the plan names (§2.5). The by-inspection
// class is listed by the matrix, not by the harness.
type Outcome string

const (
	OutcomePass         Outcome = "pass"
	OutcomeFail         Outcome = "fail"
	OutcomeNotExercised Outcome = "not-exercised"
	// OutcomeAdvisory marks a SHOULD the implementation does not
	// follow. Not a failure, and reported so the choice is visible.
	OutcomeAdvisory Outcome = "advisory"
)

type entry struct {
	Outcome Outcome  `json:"outcome"`
	Detail  []string `json:"detail,omitempty"`
}

// Report accumulates per-assertion outcomes across every fixture a run
// touches. Fail outranks pass, and pass outranks not-exercised, so an
// assertion exercised on one fixture and not another reads as
// exercised.
type Report struct {
	mu      sync.Mutex
	entries map[string]*entry
	// byFixture records the best outcome per (fixture, id), for the
	// transport-parity row. fixture is set by the test driver.
	fixture   string
	byFixture map[string]map[string]Outcome
}

var report = &Report{entries: map[string]*entry{}, byFixture: map[string]map[string]Outcome{}}

// setFixture names the fixture the following records belong to.
func (r *Report) setFixture(name string) {
	r.mu.Lock()
	r.fixture = name
	r.mu.Unlock()
}

func (r *Report) record(id string, o Outcome, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fixture != "" {
		m, ok := r.byFixture[r.fixture]
		if !ok {
			m = map[string]Outcome{}
			r.byFixture[r.fixture] = m
		}
		if rank(o) > rank(m[id]) {
			m[id] = o
		}
	}
	e, ok := r.entries[id]
	if !ok {
		e = &entry{Outcome: o}
		r.entries[id] = e
	}
	if rank(o) > rank(e.Outcome) {
		e.Outcome = o
	}
	if detail != "" {
		e.Detail = append(e.Detail, detail)
	}
}

func rank(o Outcome) int {
	switch o {
	case OutcomeFail:
		return 3
	case OutcomeAdvisory:
		return 2
	case OutcomePass:
		return 1
	}
	return 0
}

// Advisory records that a SHOULD was not followed, without failing.
func Advisory(t testing.TB, id string, format string, args ...any) {
	t.Helper()
	msg := fmt.Sprintf(format, args...)
	report.record(id, OutcomeAdvisory, msg)
	t.Logf("%s: advisory: %s", id, msg)
}

// Pass records id as passed on this fixture. detail is free text for
// the report, such as a measured number.
func Pass(t testing.TB, id string, format string, args ...any) {
	t.Helper()
	report.record(id, OutcomePass, fmt.Sprintf(format, args...))
}

// Fail records id as failed and fails the test.
func Fail(t testing.TB, id string, format string, args ...any) {
	t.Helper()
	msg := fmt.Sprintf(format, args...)
	report.record(id, OutcomeFail, msg)
	t.Errorf("%s: %s", id, msg)
}

// Check records pass or fail on cond.
func Check(t testing.TB, id string, cond bool, format string, args ...any) bool {
	t.Helper()
	if cond {
		Pass(t, id, "")
	} else {
		Fail(t, id, format, args...)
	}
	return cond
}

// NotExercised records that this fixture cannot reach id (a fixture
// item is absent, or the row belongs to the other space mode).
func NotExercised(t testing.TB, id string, why string) {
	t.Helper()
	report.record(id, OutcomeNotExercised, why)
}

// WriteJSON writes the report keyed by assertion id, ids sorted.
func (r *Report) WriteJSON(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.entries))
	for id := range r.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make(map[string]*entry, len(ids))
	for _, id := range ids {
		out[id] = r.entries[id]
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
