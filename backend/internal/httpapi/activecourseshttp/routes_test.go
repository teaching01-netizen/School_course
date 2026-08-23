package activecourseshttp

import (
	"net/url"
	"testing"
)

func TestParseSessionTimeFilters(t *testing.T) {
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
			from, to, err := parseSessionTimeFilters(parseQuery(tt.query))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
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

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
		wantPaging bool
		wantErr    bool
	}{
		{name: "legacy", query: "", wantPaging: false},
		{name: "defaults", query: "limit=25", wantLimit: 25, wantOffset: 0, wantPaging: true},
		{name: "bounded", query: "limit=200&offset=3", wantLimit: 200, wantOffset: 3, wantPaging: true},
		{name: "limit too large", query: "limit=201", wantErr: true},
		{name: "negative offset", query: "offset=-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, offset, paging, err := parsePagination(parseQuery(tt.query))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if limit != tt.wantLimit || offset != tt.wantOffset || paging != tt.wantPaging {
				t.Fatalf("got limit=%d offset=%d paging=%t", limit, offset, paging)
			}
		})
	}
}

func parseQuery(raw string) url.Values {
	query, err := url.ParseQuery(raw)
	if err != nil {
		panic(err)
	}
	return query
}
