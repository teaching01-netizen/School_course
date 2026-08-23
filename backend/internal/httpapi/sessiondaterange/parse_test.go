package sessiondaterange

import (
	"net/url"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantFrom string
		wantTo   string
		wantErr  bool
	}{
		{name: "both bounds", query: "session_date_from=2026-06-01&session_date_to=2026-06-30", wantFrom: "2026-06-01", wantTo: "2026-06-30"},
		{name: "from only", query: "session_date_from=2026-06-01", wantFrom: "2026-06-01"},
		{name: "invalid date", query: "session_date_from=2026-02-30", wantErr: true},
		{name: "reversed", query: "session_date_from=2026-06-30&session_date_to=2026-06-01", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			from, to, parseErr := Parse(query)
			if (parseErr != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", parseErr, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if from != tt.wantFrom || to != tt.wantTo {
				t.Fatalf("got from=%q to=%q, want from=%q to=%q", from, to, tt.wantFrom, tt.wantTo)
			}
		})
	}
}
