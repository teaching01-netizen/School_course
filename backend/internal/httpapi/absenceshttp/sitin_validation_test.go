package absenceshttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteSitInValidationError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"policy", errInvalidSitInSelection, 400, "invalid_sessions"},
		{"timeout", fmt.Errorf("private details: %w", context.DeadlineExceeded), 504, "timeout"},
		{"canceled", fmt.Errorf("private details: %w", context.Canceled), 503, "canceled"},
		{"database", fmt.Errorf("private details"), 500, "internal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			(&server{}).writeSitInValidationError(w, tc.err)
			var body struct{ Code, Message string }
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body.Code != tc.code || body.Message == "" || strings.Contains(body.Message, tc.err.Error()) || strings.Contains(body.Message, "private") {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status >= 500 && !strings.Contains(body.Message, "try again") {
				t.Fatalf("server error must offer a retry: %s", body.Message)
			}
		})
	}
}
