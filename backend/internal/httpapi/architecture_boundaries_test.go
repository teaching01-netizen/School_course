package httpapi

import (
	"os"
	"strings"
	"testing"
)

func TestAbsencesHTTPSubmissionHelpersDelegateDomainPolicy(t *testing.T) {
	source, err := os.ReadFile("absenceshttp/submission_helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, `"warwick-institute/internal/absences"`) {
		t.Fatalf("submission helpers should delegate absence policy to internal/absences")
	}
	for _, forbidden := range []string{`"net/mail"`, `validPlainEmailAddress`, `sessionTimingPolicyError`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("submission_helpers.go still owns domain policy marker %q", forbidden)
		}
	}
}
