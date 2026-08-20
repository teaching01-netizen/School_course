package httpadapter

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadBodyBytesRejectsBodyAboveTwoMiB(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/absences", bytes.NewReader(bytes.Repeat([]byte{'x'}, 2*1024*1024+1)))
	if _, err := (Adapter{}).ReadBodyBytes(r); !errors.Is(err, ErrRequestBodyTooLarge) {
		t.Fatalf("ReadBodyBytes error = %v, want ErrRequestBodyTooLarge", err)
	}
}

func TestDecodeJSONRejectsMalformedJSONAfterBoundedRead(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/absences", bytes.NewBufferString(`{"items":[}`))
	if err := (Adapter{}).DecodeJSON(httptest.NewRecorder(), r, &struct{}{}); err == nil {
		t.Fatal("DecodeJSON accepted malformed JSON")
	}
}
