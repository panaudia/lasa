package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var idRe = regexp.MustCompile(`\[([CPD]-[0-9.]+-[0-9]+)\]`)

// TestMatrixMatchesSpec holds three things: every assertion id in the
// three spec documents appears exactly once there, every id in the spec
// has a matrix row, and every matrix row has an id in the spec.
func TestMatrixMatchesSpec(t *testing.T) {
	spec := map[string]int{}
	for _, f := range []string{"lasa-core.md", "lasa-base-profile.md", "lasa-diagnostics.md"} {
		b, err := os.ReadFile(filepath.Join("..", f))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range idRe.FindAllStringSubmatch(string(b), -1) {
			spec[m[1]]++
		}
	}
	for id, n := range spec {
		if n != 1 {
			t.Errorf("%s appears %d times in the spec", id, n)
		}
	}

	b, err := os.ReadFile("matrix.md")
	if err != nil {
		t.Fatal(err)
	}
	matrix := map[string]bool{}
	rowRe := regexp.MustCompile(`^\| ([CPD]-[0-9.]+-[0-9]+) \|`)
	for _, line := range strings.Split(string(b), "\n") {
		if m := rowRe.FindStringSubmatch(line); m != nil {
			if matrix[m[1]] {
				t.Errorf("%s has two matrix rows", m[1])
			}
			matrix[m[1]] = true
		}
	}

	var missing, extra []string
	for id := range spec {
		if !matrix[id] {
			missing = append(missing, id)
		}
	}
	for id := range matrix {
		if _, ok := spec[id]; !ok {
			extra = append(extra, id)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("in spec, not in matrix: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("in matrix, not in spec: %v", extra)
	}
	if len(spec) == 0 {
		t.Fatal("no assertion ids found in the spec")
	}
	t.Logf("%d assertions", len(spec))
}
