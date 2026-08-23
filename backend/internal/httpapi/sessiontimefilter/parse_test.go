package sessiontimefilter

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
		{name: "both bounds", query: "session_from=09:00&session_to=11:30", wantFrom: "09:00", wantTo: "11:30"},
		{name: "from only", query: "session_from=09:00", wantFrom: "09:00"},
		{name: "invalid time", query: "session_from=9am", wantErr: true},
		{name: "reversed", query: "session_from=12:00&session_to=09:00", wantErr: true},
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
