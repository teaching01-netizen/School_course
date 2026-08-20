package clientip

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// Resolver applies one trusted-proxy policy to every request that needs a
// client identity. Forwarded headers are ignored unless the direct peer is a
// configured trusted proxy network.
type Resolver struct {
	trusted []*net.IPNet
}

func NewResolver(rawCIDRs string) (*Resolver, error) {
	resolver := &Resolver{}
	for _, raw := range strings.Split(rawCIDRs, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy CIDR %q: %w", raw, err)
		}
		resolver.trusted = append(resolver.trusted, network)
	}
	return resolver, nil
}

func (r *Resolver) Resolve(req *http.Request) string {
	if req == nil {
		return "unknown"
	}
	remote := normalizeAddress(req.RemoteAddr)
	if remote == "" {
		remote = "unknown"
	}
	if r == nil || !r.isTrusted(remote) {
		return remote
	}

	forwarded := strings.Split(req.Header.Get("X-Forwarded-For"), ",")
	// Walk from the application outward. Every configured proxy hop is
	// trusted; the first non-trusted address is the originating client.
	for i := len(forwarded) - 1; i >= 0; i-- {
		candidate := normalizeAddress(forwarded[i])
		if candidate == "" {
			continue
		}
		if net.ParseIP(candidate) == nil {
			return remote
		}
		if !r.isTrusted(candidate) {
			return candidate
		}
	}
	return remote
}

func (r *Resolver) isTrusted(address string) bool {
	ip := net.ParseIP(address)
	if ip == nil {
		return false
	}
	for _, network := range r.trusted {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func normalizeAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(raw, "[]")
}
