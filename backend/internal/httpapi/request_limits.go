package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"warwick-institute/internal/httpapi/httpadapter"
)

const documentedUploadBodyLimit int64 = 50 * 1024 * 1024

// withRequestBodyLimit rejects oversized mutation bodies before routing, auth,
// idempotency, or database work. JSON and other non-upload mutations share the
// adapter's 2 MiB limit (httpadapter.MaxJSONBodyBytes). The CRM upload endpoint
// is the only larger exception.
func withRequestBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, limited := mutationBodyLimit(r)
		if !limited {
			next.ServeHTTP(w, r)
			return
		}
		if r.ContentLength > limit {
			writeRequestTooLarge(w, limit)
			return
		}

		if limit == documentedUploadBodyLimit {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
			return
		}

		// Read and re-wrap the bounded body so chunked/unknown-length requests
		// are rejected before any downstream handler can authenticate or mutate.
		_, err := httpadapter.ReadBodyWithLimit(r, limit)
		if err != nil {
			if errors.Is(err, httpadapter.ErrRequestBodyTooLarge) {
				writeRequestTooLarge(w, limit)
			} else {
				writeRequestBodyError(w)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mutationBodyLimit(r *http.Request) (int64, bool) {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return 0, false
	}
	if r.URL.Path == "/api/v1/crm/upload" {
		return documentedUploadBodyLimit, true
	}
	return httpadapter.MaxJSONBodyBytes, true
}

func writeRequestTooLarge(w http.ResponseWriter, limit int64) {
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    "request_too_large",
		"message": "Request body exceeds the configured size limit",
	})
}

func writeRequestBodyError(w http.ResponseWriter) {
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    "bad_request",
		"message": "Request body could not be read",
	})
}
