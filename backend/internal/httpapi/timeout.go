package httpapi

import (
	"context"
	"net/http"
	"time"
)

// withRequestTimeout wraps an http.Handler with per-request context deadlines.
//
// This prevents connection pool saturation by ensuring that no request
// can indefinitely hold a database connection. The context attached to
// the request is cancelled after the timeout, propagating cancellation
// to all downstream DB calls so connections are returned to the pool.
func withRequestTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeout := 15 * time.Second // default for writes
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			timeout = 10 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}
