package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestResolverIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	resolver, err := NewResolver("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.10:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := resolver.Resolve(req); got != "192.0.2.10" {
		t.Fatalf("resolved IP = %q, want direct peer 192.0.2.10", got)
	}
}

func TestResolverUsesFirstUntrustedAddressFromTrustedChain(t *testing.T) {
	resolver, err := NewResolver("10.0.0.0/8, 192.0.2.0/24")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.8:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 192.0.2.15")
	if got := resolver.Resolve(req); got != "198.51.100.7" {
		t.Fatalf("resolved IP = %q, want originating client 198.51.100.7", got)
	}
}

func TestResolverFallsBackToPeerWhenForwardedChainIsEmptyOrTrusted(t *testing.T) {
	resolver, err := NewResolver("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	for _, forwarded := range []string{"", "10.0.0.9"} {
		t.Run(forwarded, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "10.0.0.8:4567"
			req.Header.Set("X-Forwarded-For", forwarded)
			if got := resolver.Resolve(req); got != "10.0.0.8" {
				t.Fatalf("resolved IP = %q, want trusted peer fallback 10.0.0.8", got)
			}
		})
	}
}
func TestResolverDoesNotUseMalformedForwardedAddressAsLimiterKey(t *testing.T) {
	resolver, err := NewResolver("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.8:4567"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := resolver.Resolve(req); got != "10.0.0.8" {
		t.Fatalf("resolved IP = %q, want trusted peer fallback", got)
	}
}

func TestNewResolverRejectsInvalidCIDR(t *testing.T) {
	if _, err := NewResolver("not-a-cidr"); err == nil {
		t.Fatal("NewResolver accepted invalid CIDR")
	}
}
