package emailnotifierhttp

import (
	"errors"
	"testing"
)

func TestValidateTemplateFieldsRejectsBlankSubject(t *testing.T) {
	_, _, _, err := validateTemplateFields("Sit-in", "   ", "Body")
	if !errors.Is(err, errTemplateValidation) {
		t.Fatalf("err = %v, want template validation error", err)
	}
	if got := templateValidationMessage(err); got != "subject is required" {
		t.Fatalf("message = %q, want subject is required", got)
	}
}

func TestValidateTemplateFieldsTrimsAcceptedValues(t *testing.T) {
	name, subject, body, err := validateTemplateFields(" Sit-in ", " Subject ", " Body ")
	if err != nil {
		t.Fatalf("validateTemplateFields returned error: %v", err)
	}
	if name != "Sit-in" || subject != "Subject" || body != "Body" {
		t.Fatalf("got (%q, %q, %q), want trimmed values", name, subject, body)
	}
}

func TestValidateTemplateContentRejectsBlankBody(t *testing.T) {
	_, _, err := validateTemplateContent("Subject", "\t")
	if !errors.Is(err, errTemplateValidation) {
		t.Fatalf("err = %v, want template validation error", err)
	}
	if got := templateValidationMessage(err); got != "body is required" {
		t.Fatalf("message = %q, want body is required", got)
	}
}
