package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultMaxBodyBytes int64 = 2 << 20
const defaultMinRequestInterval = 500 * time.Millisecond
const defaultCircuitBreakerFailures = 3
const defaultCircuitBreakerCooldown = 10 * time.Second
const defaultAuthenticationCooldown = 10 * time.Second
const maxBodyBytesUpperBound int64 = 16 << 20

// defaultEstimatedBodyBytes is the per-request egress reservation made at
// admission time, before any body has been read. It bounds how far a
// concurrent burst can overshoot the byte budget in a single window.
const defaultEstimatedBodyBytes int64 = 256 << 10

type Config struct {
	BaseURL                 string
	Username                string
	Password                string
	HTTPClient              *http.Client
	MaxBodyBytes            int64
	MaxConcurrent           int
	MinRequestInterval      time.Duration
	CircuitBreakerFailures  int
	CircuitBreakerCooldown  time.Duration
	AuthenticationCooldown  time.Duration
	MaxRequestsPerMinute    int
	MaxEgressBytesPerMinute int64
	// EstimatedBodyBytes is the per-request egress reservation charged at
	// admission time, before any response body is read (see reserveBudget).
	// <= 0 selects the default.
	EstimatedBodyBytes int64
}

type Response struct {
	StatusCode int
	Path       string
	Body       []byte
}

type Client struct {
	baseURL            string
	username           string
	password           string
	httpClient         *http.Client
	maxBodyBytes       int64
	semaphore          chan struct{}
	rateMu             sync.Mutex
	nextRequestAt      time.Time
	minRequestInterval time.Duration
	authMu             sync.Mutex
	authenticated      bool
	authGeneration     uint64
	authFailureUntil   time.Time
	authCooldown       time.Duration
	breakerMu          sync.Mutex
	breakerFailures    int
	breakerThreshold   int
	breakerOpenUntil   time.Time
	breakerCooldown    time.Duration
	bucketMu           sync.Mutex
	bucketWindowStart  time.Time
	bucketReqCount     int
	bucketByteCount    int64
	bucketPendingBytes int64
	bucketEstimate     int64
	bucketMaxReq       int
	bucketMaxBytes     int64
	now                func() time.Time
}

func New(config Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, &ConfigError{Message: "base URL must be absolute"}
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second, Jar: jar}
	} else if httpClient.Jar == nil {
		httpClient.Jar = jar
	}
	originalRedirect := httpClient.CheckRedirect
	httpClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := validateRedirect(parsed, request); err != nil {
			return err
		}
		if originalRedirect != nil {
			return originalRedirect(request, via)
		}
		return nil
	}
	maxBodyBytes := config.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	if maxBodyBytes > maxBodyBytesUpperBound {
		maxBodyBytes = maxBodyBytesUpperBound
	}
	maxConcurrent := config.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 32
	}
	if maxConcurrent > 128 {
		maxConcurrent = 128
	}
	estimate := config.EstimatedBodyBytes
	if estimate <= 0 {
		estimate = defaultEstimatedBodyBytes
	}
	minRequestInterval := config.MinRequestInterval
	if minRequestInterval < 0 {
		minRequestInterval = defaultMinRequestInterval
	}
	breakerThreshold := config.CircuitBreakerFailures
	if breakerThreshold <= 0 {
		breakerThreshold = defaultCircuitBreakerFailures
	}
	breakerCooldown := config.CircuitBreakerCooldown
	if breakerCooldown <= 0 {
		breakerCooldown = defaultCircuitBreakerCooldown
	}
	authCooldown := config.AuthenticationCooldown
	if authCooldown <= 0 {
		authCooldown = defaultAuthenticationCooldown
	}
	c := &Client{
		baseURL: baseURL, username: config.Username, password: config.Password,
		httpClient: httpClient, maxBodyBytes: maxBodyBytes,
		semaphore: make(chan struct{}, maxConcurrent), minRequestInterval: minRequestInterval,
		breakerThreshold: breakerThreshold, breakerCooldown: breakerCooldown,
		authCooldown: authCooldown,
		bucketMaxReq: config.MaxRequestsPerMinute, bucketMaxBytes: config.MaxEgressBytesPerMinute,
		bucketEstimate: estimate, now: time.Now,
	}
	return c, nil
}

func validateRedirect(base *url.URL, next *http.Request) error {
	if base == nil || next == nil || next.URL == nil {
		return ErrUnsafeEndpoint
	}
	if next.URL.Scheme != base.Scheme || next.URL.Host != base.Host {
		return ErrUnsafeEndpoint
	}
	path := strings.ToLower(next.URL.Path)
	for _, blocked := range []string{"confirm", "delete", "import", "edit", "new", "checkin"} {
		if strings.Contains(path, blocked) {
			return ErrUnsafeEndpoint
		}
	}
	if next.Method != http.MethodGet {
		return ErrUnsafeEndpoint
	}
	return nil
}

func (c *Client) Do(ctx context.Context, request Request) (Response, error) {
	if !allowed(request) {
		return Response{}, ErrUnsafeEndpoint
	}
	if err := c.checkCircuit(); err != nil {
		return Response{}, err
	}
	if err := c.acquire(ctx); err != nil {
		return Response{}, err
	}
	defer c.release()
	if err := c.ensureAuthenticated(ctx); err != nil {
		c.recordFailure(err)
		return Response{}, err
	}
	generation := c.AuthGeneration()
	response, err := c.doOnce(ctx, request, generation)
	if err == nil {
		c.recordSuccess()
		return response, nil
	}
	c.recordFailure(err)
	if !errors.Is(err, ErrSessionExpired) {
		return Response{}, err
	}
	if err := c.reauthenticate(ctx, generation); err != nil {
		c.recordFailure(err)
		return Response{}, err
	}
	response, err = c.doOnce(ctx, request, c.AuthGeneration())
	if err != nil {
		c.recordFailure(err)
		return Response{}, err
	}
	c.recordSuccess()
	return response, nil
}

func (c *Client) checkCircuit() error {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	if c.breakerOpenUntil.IsZero() {
		return nil
	}
	if time.Now().Before(c.breakerOpenUntil) {
		return ErrCircuitOpen
	}
	c.breakerOpenUntil = time.Time{}
	c.breakerFailures = 0
	return nil
}

func (c *Client) CircuitState() (bool, time.Time) {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	if c.breakerOpenUntil.IsZero() || time.Now().After(c.breakerOpenUntil) {
		return false, time.Time{}
	}
	return true, c.breakerOpenUntil
}

// BudgetExceeded reports whether the per-minute egress budget (requests or
// bytes) has been exhausted for the current window. The legacy runner uses
// this to pause enqueueing new work while the budget is spent; an expired
// window rolls forward here too, so the pause cannot stick once the minute
// has passed even if no further requests flow.
func (c *Client) BudgetExceeded() bool {
	if c.bucketMaxReq <= 0 && c.bucketMaxBytes <= 0 {
		return false
	}
	c.bucketMu.Lock()
	defer c.bucketMu.Unlock()
	c.resetWindowLocked(c.now())
	if c.bucketMaxReq > 0 && c.bucketReqCount >= c.bucketMaxReq {
		return true
	}
	return c.bucketMaxBytes > 0 && c.bucketByteCount+c.bucketPendingBytes >= c.bucketMaxBytes
}

func (c *Client) EgressStats() (int, int64, time.Time) {
	c.bucketMu.Lock()
	defer c.bucketMu.Unlock()
	resetAt := c.bucketWindowStart.Add(time.Minute)
	if c.bucketWindowStart.IsZero() {
		resetAt = c.now().Add(time.Minute)
	}
	return c.bucketReqCount, c.bucketByteCount, resetAt
}

// resetWindowLocked rolls the per-minute budget window forward when it has
// expired. Callers must hold bucketMu.
func (c *Client) resetWindowLocked(now time.Time) {
	if c.bucketWindowStart.IsZero() || now.Sub(c.bucketWindowStart) >= time.Minute {
		c.bucketWindowStart = now
		c.bucketReqCount = 0
		c.bucketByteCount = 0
		c.bucketPendingBytes = 0
	}
}

func (c *Client) reserveBudget() error {
	if c.bucketMaxReq <= 0 && c.bucketMaxBytes <= 0 {
		return nil
	}
	c.bucketMu.Lock()
	defer c.bucketMu.Unlock()
	c.resetWindowLocked(c.now())
	if c.bucketMaxReq > 0 && c.bucketReqCount+1 > c.bucketMaxReq {
		return &EgressBudgetError{ResetAt: c.bucketWindowStart.Add(time.Minute)}
	}
	// Admit only when recorded bytes plus the reservations of every in-flight
	// request plus this request's estimate stay within the byte budget; a
	// concurrent burst cannot push egress past the budget before any body has
	// been read.
	if c.bucketMaxBytes > 0 && c.bucketByteCount+c.bucketPendingBytes+c.bucketEstimate > c.bucketMaxBytes {
		return &EgressBudgetError{ResetAt: c.bucketWindowStart.Add(time.Minute)}
	}
	c.bucketReqCount++
	c.bucketPendingBytes += c.bucketEstimate
	return nil
}

// releaseBudgetEstimate returns the reservation made by reserveBudget once
// the request has finished, whether or not any body was read. Every admitted
// request must release exactly once.
func (c *Client) releaseBudgetEstimate() {
	if c.bucketMaxBytes <= 0 {
		return
	}
	c.bucketMu.Lock()
	c.bucketPendingBytes -= c.bucketEstimate
	if c.bucketPendingBytes < 0 {
		c.bucketPendingBytes = 0
	}
	c.bucketMu.Unlock()
}

func (c *Client) recordEgressBytes(n int) {
	if n <= 0 {
		return
	}
	c.bucketMu.Lock()
	if c.bucketWindowStart.IsZero() {
		c.bucketWindowStart = c.now()
	}
	c.bucketByteCount += int64(n)
	c.bucketMu.Unlock()
}

func (c *Client) recordFailure(err error) {
	if !errors.Is(err, ErrSourceUnavailable) && !errors.Is(err, ErrRateLimited) {
		return
	}
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	c.breakerFailures++
	if c.breakerFailures >= c.breakerThreshold {
		c.breakerOpenUntil = time.Now().Add(c.breakerCooldown)
	}
}

func (c *Client) recordSuccess() {
	c.breakerMu.Lock()
	c.breakerFailures = 0
	c.breakerOpenUntil = time.Time{}
	c.breakerMu.Unlock()
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.semaphore <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) release() { <-c.semaphore }

// MaxConcurrent returns the configured in-flight request cap, the size of
// the semaphore that bounds concurrent Do calls.
func (c *Client) MaxConcurrent() int { return cap(c.semaphore) }

func (c *Client) ensureAuthenticated(ctx context.Context) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.authenticated {
		return nil
	}
	if c.authFailureBlocked() {
		return ErrAuthentication
	}
	if err := c.loginLocked(ctx); err != nil {
		return c.rememberAuthFailure(err)
	}
	return nil
}

func (c *Client) reauthenticate(ctx context.Context, observedGeneration uint64) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if c.authenticated && c.authGeneration != observedGeneration {
		return nil
	}
	if c.authFailureBlocked() {
		return ErrAuthentication
	}
	if err := c.loginLocked(ctx); err != nil {
		return c.rememberAuthFailure(err)
	}
	return nil
}

func (c *Client) Login(ctx context.Context) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	if err := c.loginLocked(ctx); err != nil {
		return c.rememberAuthFailure(err)
	}
	return nil
}

func (c *Client) authFailureBlocked() bool {
	return !c.authFailureUntil.IsZero() && time.Now().Before(c.authFailureUntil)
}

func (c *Client) rememberAuthFailure(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	c.authFailureUntil = time.Now().Add(c.authCooldown)
	return err
}

func (c *Client) loginLocked(ctx context.Context) error {
	tokenResponse, err := c.request(ctx, http.MethodGet, "/Account/Login", nil, nil)
	if err != nil {
		return fmt.Errorf("get login page: %w", err)
	}
	token, err := requestToken(tokenResponse.Body)
	if err != nil {
		return fmt.Errorf("parse login page: %w", err)
	}
	if !c.hasAntiforgeryCookie() {
		return ErrAuthentication
	}
	form := url.Values{"Input.UserName": {c.username}, "Input.Password": {c.password}, "__RequestVerificationToken": {token}}
	response, err := c.request(ctx, http.MethodPost, "/Account/Login", nil, form)
	if err != nil {
		return fmt.Errorf("post login: %w", err)
	}
	if bytesContainFold(response.Body, "invalid login") || bytesContainFold(response.Body, "login") && !bytesContainFold(response.Body, "logout") {
		return ErrAuthentication
	}
	if !c.hasSessionCookie() {
		return ErrAuthentication
	}
	c.authenticated = true
	c.authFailureUntil = time.Time{}
	c.authGeneration++
	return nil
}

func (c *Client) hasSessionCookie() bool {
	if c.httpClient == nil || c.httpClient.Jar == nil {
		return false
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return false
	}
	for _, cookie := range c.httpClient.Jar.Cookies(base) {
		name := strings.ToLower(cookie.Name)
		if strings.Contains(name, "antiforgery") || strings.Contains(name, "verification") {
			continue
		}
		return true
	}
	return false
}

func (c *Client) hasAntiforgeryCookie() bool {
	if c.httpClient == nil || c.httpClient.Jar == nil {
		return false
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return false
	}
	for _, cookie := range c.httpClient.Jar.Cookies(base) {
		name := strings.ToLower(cookie.Name)
		if strings.Contains(name, "antiforgery") || strings.Contains(name, "verification") {
			return true
		}
	}
	return false
}

func (c *Client) doOnce(ctx context.Context, request Request, observedGeneration uint64) (Response, error) {
	response, err := c.request(ctx, request.Method, request.Path, request.Query, request.Form)
	if err != nil {
		return Response{}, err
	}
	if isLoginPage(response.Body) {
		c.markUnauthenticated(observedGeneration)
		return Response{}, ErrSessionExpired
	}
	return response, nil
}

// AuthGeneration returns how many times the client has successfully logged
// in. Session-scoped caches (e.g. parsed antiforgery tokens) become invalid
// on every login because the site re-issues its antiforgery cookie then.
func (c *Client) AuthGeneration() uint64 {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	return c.authGeneration
}

func (c *Client) markUnauthenticated(observedGeneration uint64) {
	c.authMu.Lock()
	if c.authGeneration == observedGeneration {
		c.authenticated = false
	}
	c.authMu.Unlock()
}
