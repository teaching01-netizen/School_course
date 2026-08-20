package parser

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Banned literals extracted from the LIVE capture legacy.html (repo
// root). The parser testdata fixtures must never contain them:
//
//   - "PeeRD!" — the logged-in username, legacy.html line 45
//     ("<a title=\"Manage\" href=\"/Account/Manage\">Hello PeeRD!</a>").
//   - The __RequestVerificationToken value, legacy.html lines 49, 81 and
//     14761 (identical in all three).
//   - "Nichakarn Tuntiwiwat (Ame)" — a real student name from the
//     attendee sub-row, legacy.html line 277.
//
// The trimmed course_list.html fixture is extracted from the table slice
// only (lines 84-314 + closing tags), so the username and token (navbar /
// form region) are not present in it; the student identity was replaced
// with "Test Student (TS)". This test guards all three regardless.
func TestFixtures_ContainNoSecrets(t *testing.T) {
	banned := []string{
		"PeeRD!",
		"CfDJ8Gwe0P-1zWlPpuDI7l8Fb7MTFZ3u_Uu2OVyvGm-GboQKr1YHc_cgtiDVcigDmUynGyZuANzaYi3bsr6znxZoORwbUW5s3x1-pdzNYM3zXzxI0IOxVkKfRLzhxgJcdhcIMD29GY7NRMLDVbfVdyIYzLTSDA5zdhf7MqS7fmXZ5yn53yiK8hAn39kXo6KTcwfECw",
		"Nichakarn Tuntiwiwat (Ame)",
	}
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("reading testdata: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join("testdata", e.Name()))
		if err != nil {
			t.Fatalf("reading testdata/%s: %v", e.Name(), err)
		}
		for _, lit := range banned {
			if bytes.Contains(b, []byte(lit)) {
				t.Errorf("testdata/%s contains banned literal %q", e.Name(), lit)
			}
		}
	}
}
