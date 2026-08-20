package parser

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateGoldens writes golden files instead of comparing when
// `go test ./internal/legacysync/parser -update` is passed.
var updateGoldens = flag.Bool("update", false, "update golden files in testdata")

func goldenPath(name string) string {
	return filepath.Join("testdata", name)
}

// readGolden returns the golden bytes for name, failing the test if the
// golden is missing (never silently pass).
func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("golden file %s: %v (run `go test ./internal/legacysync/parser -update` to create it)", goldenPath(name), err)
	}
	return b
}

// checkGolden compares got against the golden for name; with -update it
// writes got instead.
func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := goldenPath(name)
	if *updateGoldens {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("writing golden %s: %v", path, err)
		}
		return
	}
	want := readGolden(t, name)
	if !bytes.Equal(got, want) {
		t.Errorf("golden %s mismatch:\ngot  %s\nwant %s", path, got, want)
	}
}
