package absenceshttp

import "testing"

func TestSpreadsheetSafeCSVCellPrefixesFormulaLeaders(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "equals", value: "=HYPERLINK(\"https://attacker.invalid\")", want: "'=HYPERLINK(\"https://attacker.invalid\")"},
		{name: "plus", value: "+cmd", want: "'+cmd"},
		{name: "minus", value: "-2+3", want: "'-2+3"},
		{name: "at", value: "@SUM(A1:A2)", want: "'@SUM(A1:A2)"},
		{name: "leading whitespace", value: " \t=1+1", want: "' \t=1+1"},
		{name: "ordinary text", value: "Alex Smith", want: "Alex Smith"},
		{name: "empty", value: "", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := spreadsheetSafeCSVCell(test.value); got != test.want {
				t.Fatalf("spreadsheetSafeCSVCell(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestSpreadsheetSafeCSVRowSanitizesEveryExportedCell(t *testing.T) {
	row := []string{"=name", "+subject", "-reason", "@course", "ordinary"}
	got := spreadsheetSafeCSVRow(row)
	for i, want := range []string{"'=name", "'+subject", "'-reason", "'@course", "ordinary"} {
		if got[i] != want {
			t.Fatalf("row[%d] = %q, want %q", i, got[i], want)
		}
	}
}
