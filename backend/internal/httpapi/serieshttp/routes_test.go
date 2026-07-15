package serieshttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/series"
)

func TestWriteRecurrenceValidationErr(t *testing.T) {
	recorder := httptest.NewRecorder()
	s := &server{a: httpadapter.New(nil, nil)}
	err := fmt.Errorf("wrapped: %w", &series.ValidationError{Code: "count_exceeds_limit", Message: "count must be at most 1000"})

	if !s.writeRecurrenceValidationErr(recorder, err) {
		t.Fatal("validation error was not handled")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "invalid_recurrence" {
		t.Fatalf("code = %q, want invalid_recurrence", body.Code)
	}
	if body.Message != "count must be at most 1000" {
		t.Fatalf("message = %q", body.Message)
	}
}
