package absenceshttp

import "testing"

func TestNormalizeWCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "lowercase preserved", input: "w12345", want: "w12345"},
		{name: "uppercase lowered", input: "W12345", want: "w12345"},
		{name: "mixed case lowered", input: "W12345AbC", want: "w12345abc"},
		{name: "leading/trailing spaces trimmed", input: "  W12345  ", want: "w12345"},
		{name: "internal spaces preserved", input: "W 12345", want: "w 12345"},
		{name: "empty string unchanged", input: "", want: ""},
		{name: "only spaces becomes empty", input: "   ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWCode(tt.input)
			if got != tt.want {
				t.Errorf("normalizeWCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
