package crossstudy

import "testing"

func TestNormalizeWCodeAcceptsCRMExportCapitalization(t *testing.T) {
	tests := map[string]string{
		"W240591":     "w240591",
		"w240591":     "w240591",
		"  W240591  ": "w240591",
	}

	for input, want := range tests {
		if got := normalizeWCode(input); got != want {
			t.Errorf("normalizeWCode(%q) = %q, want %q", input, got, want)
		}
	}
}
