package absenceshttp

import (
	"net/http/httptest"
	"testing"

	"warwick-institute/internal/clientip"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

func TestPublicRateLimitIPIgnoresSpoofedForwardedHeader(t *testing.T) {
	resolver, err := clientip.NewResolver("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{deps: httpdeps.Deps{ClientIP: resolver}, a: httpadapter.Adapter{}}
	req := httptest.NewRequest("POST", "/api/v1/absence-self-service/parent-verification/send", nil)
	req.RemoteAddr = "192.0.2.10:4000"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := s.requestIP(req); got != "192.0.2.10" {
		t.Fatalf("requestIP = %q, want direct peer", got)
	}
}

func TestPublicRateLimitIPUsesForwardedAddressOnlyThroughTrustedProxy(t *testing.T) {
	resolver, err := clientip.NewResolver("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{deps: httpdeps.Deps{ClientIP: resolver}, a: httpadapter.Adapter{}}
	req := httptest.NewRequest("POST", "/api/v1/absence-self-service/parent-verification/send", nil)
	req.RemoteAddr = "10.0.0.8:4000"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := s.requestIP(req); got != "198.51.100.7" {
		t.Fatalf("requestIP = %q, want trusted forwarded client", got)
	}
}
