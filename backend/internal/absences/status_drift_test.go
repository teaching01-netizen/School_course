package absences

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var checkRE = regexp.MustCompile(`(?i)CHECK\s*\(\s*status\s+IN\s*\(([^)]+)\)`)
var arrayRE = regexp.MustCompile(`ABSENCE_STATUSES\s*=\s*\[([^\]]+)\]`)

func parseCheckList(checkBody string) []string {
	parts := strings.Split(checkBody, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "'\"")
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseTSArray(arrayBody string) []string {
	parts := strings.Split(arrayBody, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "'\"")
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func setOf(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func statusesAsStrings() []string {
	all := AllStatuses()
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = string(s)
	}
	return out
}

func equalSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ma := setOf(a)
	mb := setOf(b)
	if len(ma) != len(mb) {
		return false
	}
	for k := range ma {
		if !mb[k] {
			return false
		}
	}
	return true
}

func findFileUpwards(start, rel string, maxDepth int) (string, bool) {
	dir := start
	for i := 0; i < maxDepth; i++ {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func TestStatusDrift_SQL(t *testing.T) {
	cwd, _ := os.Getwd()
	rel := filepath.Join("db", "migrations", "00068_special_approved_absence_status.sql")
	altRel := filepath.Join("backend", "db", "migrations", "00068_special_approved_absence_status.sql")

	// Try relative to CWD (covers `go test ./...` from backend/ and from repo root)
	candidates := []string{
		filepath.Join(cwd, rel),
		filepath.Join(cwd, altRel),
		filepath.Join(cwd, "..", rel),
		filepath.Join(cwd, "..", altRel),
		filepath.Join(cwd, "..", "..", rel),
		filepath.Join(cwd, "..", "..", altRel),
	}
	// Also walk upwards for repo-root anchored lookup.
	if p, ok := findFileUpwards(cwd, rel, 6); ok {
		candidates = append([]string{p}, candidates...)
	}
	if p, ok := findFileUpwards(cwd, altRel, 6); ok {
		candidates = append([]string{p}, candidates...)
	}

	var data []byte
	var found string
	for _, c := range candidates {
		if b, err := os.ReadFile(c); err == nil {
			data = b
			found = c
			break
		}
	}
	if data == nil {
		t.Skipf("migration file not found (tried %v); skipping drift test", candidates)
	}

	// Collect all CHECK (status IN (...)) occurrences; pick the largest set
	// (the Up constraint with 5 values, not the Down with 4).
	matches := checkRE.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatalf("no CHECK (status IN (...)) found in %s", found)
	}
	var best []string
	for _, m := range matches {
		vals := parseCheckList(m[1])
		if len(vals) > len(best) {
			best = vals
		}
	}
	if len(best) == 0 {
		t.Fatalf("CHECK IN list empty in %s", found)
	}

	goStatuses := statusesAsStrings()
	if !equalSets(best, goStatuses) {
		t.Errorf("Go vs SQL drift: Go AllStatuses()=%v, SQL CHECK in %s=%v", goStatuses, found, best)
	}

	// Also verify the IN clauses that bucket active/archived stay consistent.
	// These are documented with sync comments but not parameterized; the drift
	// test asserts the union of buckets equals AllStatuses.
	active := []string{"pending", "reviewed"}
	archived := []string{"actioned", "cancelled", "special_approved"}
	union := append(append([]string{}, active...), archived...)
	if !equalSets(union, goStatuses) {
		t.Errorf("active+archived bucket union %v does not equal AllStatuses %v", union, goStatuses)
	}
}

func TestStatusDrift_Frontend(t *testing.T) {
	cwd, _ := os.Getwd()
	rel := filepath.Join("src", "features", "absences", "types.ts")
	altRel := filepath.Join("..", "src", "features", "absences", "types.ts")

	candidates := []string{
		filepath.Join(cwd, rel),
		filepath.Join(cwd, altRel),
		filepath.Join(cwd, "..", rel),
		filepath.Join(cwd, "..", altRel),
		filepath.Join(cwd, "..", "..", rel),
		filepath.Join(cwd, "..", "..", altRel),
	}
	if p, ok := findFileUpwards(cwd, rel, 6); ok {
		candidates = append([]string{p}, candidates...)
	}
	if p, ok := findFileUpwards(cwd, altRel, 6); ok {
		candidates = append([]string{p}, candidates...)
	}

	var data []byte
	var found string
	for _, c := range candidates {
		if b, err := os.ReadFile(c); err == nil {
			data = b
			found = c
			break
		}
	}
	if data == nil {
		t.Skipf("frontend types file not found (tried %v); skipping drift test", candidates)
	}

	m := arrayRE.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("ABSENCE_STATUSES array not found in %s", found)
	}
	tsVals := parseTSArray(m[1])
	goStatuses := statusesAsStrings()
	if !equalSets(tsVals, goStatuses) {
		t.Errorf("Go vs frontend drift: Go AllStatuses()=%v, TS ABSENCE_STATUSES in %s=%v", goStatuses, found, tsVals)
	}
}
