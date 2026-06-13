package auth

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// InMemoryLoginRateLimiter implements LoginRateLimiter using in-memory token buckets.
// Intended for dev/test environments where no DB-backed ratelimit.Store is configured.
type InMemoryLoginRateLimiter struct {
	mu         sync.Mutex
	userLimit  map[string]*memLimiterEntry
	limitSizes []string
}

type memLimiterEntry struct {
	limiter   *rate.Limiter
	createdAt time.Time
}

func NewInMemoryLoginRateLimiter() *InMemoryLoginRateLimiter {
	return &InMemoryLoginRateLimiter{}
}

func (l *InMemoryLoginRateLimiter) Allow(_ context.Context, username, ip string) (RateLimitResult, error) {
	ipOK := l.allowKey("ip:" + ip)
	userOK := l.allowKey("user:" + username)
	allowed := ipOK && userOK
	return RateLimitResult{Allowed: allowed, Remaining: 0}, nil
}

func (l *InMemoryLoginRateLimiter) allowKey(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.userLimit == nil {
		l.userLimit = make(map[string]*memLimiterEntry)
	}

	l.evictLocked(time.Now())

	if entry, ok := l.userLimit[key]; ok {
		return entry.limiter.Allow()
	}

	lim := rate.NewLimiter(rate.Limit(5.0/60.0), 1)
	l.userLimit[key] = &memLimiterEntry{
		limiter:   lim,
		createdAt: time.Now(),
	}
	l.limitSizes = append(l.limitSizes, key)

	if len(l.limitSizes) > 10_000 {
		oldest := l.limitSizes[0]
		delete(l.userLimit, oldest)
		l.limitSizes = l.limitSizes[1:]
	}

	return lim.Allow()
}

func (l *InMemoryLoginRateLimiter) evictLocked(now time.Time) {
	const maxEntries = 10_000
	const ttl = 10 * time.Minute

	if l.userLimit == nil {
		return
	}

	stillAlive := l.limitSizes[:0]
	for _, key := range l.limitSizes {
		entry, ok := l.userLimit[key]
		if !ok {
			continue
		}
		if now.Sub(entry.createdAt) > ttl {
			delete(l.userLimit, key)
			continue
		}
		stillAlive = append(stillAlive, key)
	}
	l.limitSizes = stillAlive

	for len(l.limitSizes) > maxEntries {
		oldest := l.limitSizes[0]
		delete(l.userLimit, oldest)
		l.limitSizes = l.limitSizes[1:]
	}
}
