package crmimport

import "testing"

func TestNormalizeWCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "uppercase", input: "W250001", want: "w250001"},
		{name: "mixed case", input: "W250abc", want: "w250abc"},
		{name: "leading spaces", input: "  W250002", want: "w250002"},
		{name: "trailing spaces", input: "W250003  ", want: "w250003"},
		{name: "both spaces", input: "  W250004  ", want: "w250004"},
		{name: "already normalized", input: "w250005", want: "w250005"},
		{name: "empty string", input: "", want: ""},
		{name: "tabs", input: "\tW250006\t", want: "w250006"},
		{name: "mixed whitespace and case", input: "  W25ABC  ", want: "w25abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeWCode(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeWCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
