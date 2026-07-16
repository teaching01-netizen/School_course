package availabilityhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/scheduling"
)

func TestAvailabilityConflictUsesStableHTTPResponse(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	s := server{a: httpadapter.New(nil, nil)}
	s.writeAvailabilityMutationErr(recorder, &scheduling.Err{
		Code:    "availability_conflict",
		Message: "teacher availability would leave future sessions uncovered",
		Details: scheduling.ConflictDetails{
			Resource:   "teacher",
			SessionIDs: []string{"11111111-1111-1111-1111-111111111111"},
		},
	})

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusConflict)
	}
	var body struct {
		Code    string `json:"code"`
		Details struct {
			Resource   string   `json:"resource"`
			SessionIDs []string `json:"session_ids"`
		} `json:"details"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "availability_conflict" || body.Details.Resource != "teacher" || len(body.Details.SessionIDs) != 1 {
		t.Fatalf("unexpected response: %+v", body)
	}
}
