package conformance

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/panaudia/lasa/server"
)

// The suite runs against one external implementation when
// LASA_CONFORM_URL is set (FixtureFromEnv), and otherwise against the
// in-repo reference: the lasa server shell over a stub backend, in
// both modes and on both transports. LASA_CONFORM_REPORT names a JSON
// file to write the per-assertion outcomes to.

var (
	fixtureOnce sync.Once
	fixtureSet  []*Fixture
	fixtureErr  error
	closers     []func()
)

func TestMain(m *testing.M) {
	code := m.Run()
	for _, c := range closers {
		c()
	}
	if p := os.Getenv("LASA_CONFORM_REPORT"); p != "" {
		if err := report.WriteJSON(p); err != nil {
			fmt.Fprintln(os.Stderr, "report:", err)
			code = 1
		}
	}
	os.Exit(code)
}

// fixtures returns the fixtures every test iterates. Tests skip a row
// with NotExercised when a fixture cannot reach it.
func fixtures(t *testing.T) []*Fixture {
	t.Helper()
	fixtureOnce.Do(func() {
		if f, ok, err := FixtureFromEnv(); ok {
			fixtureErr = err
			fixtureSet = []*Fixture{f}
			return
		}
		for _, tr := range []Transport{TransportQUIC, TransportWebTransport} {
			for _, ticketed := range []bool{false, true} {
				f, closeFn, err := startReference(tr, ticketed, nil)
				if err != nil {
					fixtureErr = err
					return
				}
				f.Shutdown = nil // shared: the shutdown row boots its own
				fixtureSet = append(fixtureSet, f)
				closers = append(closers, closeFn)
			}
		}
	})
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
	return fixtureSet
}

// eachFixture runs fn as a subtest per fixture.
func eachFixture(t *testing.T, fn func(t *testing.T, f *Fixture)) {
	t.Helper()
	for _, f := range fixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			report.setFixture(f.Name)
			defer report.setFixture("")
			fn(t, f)
		})
	}
}

// privateReference boots a reference space for one test that needs to
// change or stop it (render order, shutdown). External fixtures cannot
// provide this, and the caller reports the rows as not exercised.
func privateReference(t *testing.T, tr Transport, ticketed bool, mod func(*server.Config)) *Fixture {
	t.Helper()
	f, closeFn, err := startReference(tr, ticketed, mod)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeFn)
	return f
}

// externalOnly reports whether the run targets an external server, in
// which case private references are unavailable.
func externalOnly() bool { return os.Getenv("LASA_CONFORM_URL") != "" }

// serverConfig lets tests tune a private reference without importing
// the server package themselves.
type serverConfig = server.Config
