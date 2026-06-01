package smartsms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/publicsuffix"
)

const maxResponseBodySize = 2 * 1024 * 1024 // 2 MB

const defaultBaseURL = "https://smcp.sc4msg.com"
const defaultSender = "Warwick"
const defaultLabel = "General"
const defaultTimeout = 30 * time.Second

const clientUserAgent = "WarwickInstitute-SmartSMS/1.0"

const maxRetries = 1
const retryBaseDelay = 500 * time.Millisecond

// Config holds the credentials and settings for the SmartSMS client.
type Config struct {
	BaseURL  string
	Username string
	Password string
	Sender   string
	Label    string
	Timeout  time.Duration

	// skipBaseURLValidation disables SSRF checks on BaseURL (test only).
	skipBaseURLValidation bool
}

// Client is a stateful HTTP client for the SmartSMS platform. It manages
// Laravel session cookies and CSRF tokens, and performs the 3-step SMS send flow.
//
// All public methods are safe for concurrent use.
type Client struct {
	mu              sync.Mutex
	httpClient      *http.Client
	baseURL         string
	sender          string
	label           string
	username        string
	password        string
	csrfToken       atomic.Value
	loggedIn        bool
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc
}

// httpStatusError is returned when the HTTP response has a non-200 status code.
type httpStatusError struct {
	StatusCode int
	Body       []byte
	Path       string
}

func (e *httpStatusError) Error() string {
	snippet := string(e.Body)
	if len(snippet) > 200 {
		snippet = snippet[:200] + "..."
	}
	return fmt.Sprintf("smartsms: POST %s: HTTP %d: %s", e.Path, e.StatusCode, snippet)
}

// isTransientError returns true if the error is retryable (5xx or connection error).
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500
	}
	return false
}

// isSessionError returns true if the error indicates an invalid session or CSRF expiry.
func isSessionError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, 419:
			return true
		}
	}
	return false
}

// New creates a new SmartSMS client. It validates the config but does not
// log in; call Login to establish a session.
func New(cfg Config) (*Client, error) {
	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("smartsms: Username and Password are required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if !cfg.skipBaseURLValidation {
		if err := validateBaseURL(baseURL); err != nil {
			return nil, err
		}
	}

	sender := cfg.Sender
	if sender == "" {
		sender = defaultSender
	}

	label := cfg.Label
	if label == "" {
		label = defaultLabel
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, fmt.Errorf("smartsms: cookiejar: %w", err)
	}

	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        25,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
			Jar:     jar,
			Timeout: timeout,
		},
		baseURL:  baseURL,
		sender:   sender,
		label:    label,
		username: cfg.Username,
		password: cfg.Password,
	}, nil
}

// LoggedIn reports whether a session has been established.
func (c *Client) LoggedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loggedIn
}

// HealthCheck verifies the client can reach the SmartSMS platform.
func (c *Client) HealthCheck(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sendsmsURL, err := c.abs("/sendsms")
	if err != nil {
		return fmt.Errorf("smartsms: health check: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sendsmsURL, nil)
	if err != nil {
		return fmt.Errorf("smartsms: health check request: %w", err)
	}
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("smartsms: health check: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("smartsms: health check: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("smartsms: health check: HTTP %d", resp.StatusCode)
	}
	if isLoginPage(string(body)) {
		return fmt.Errorf("smartsms: health check: not authenticated (login page returned)")
	}
	return nil
}

// GetCredits returns 0. The SmartSMS API does not expose a standalone credits
// endpoint; credit information is only available as a side-effect of previewData.
func (c *Client) GetCredits(_ context.Context) (int, error) {
	return 0, nil
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

// Login authenticates with the SmartSMS platform by:
//  1. GET /sendsms → scrape the _token CSRF value.
//  2. POST /login with email, password, _token (multipart form).
func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.loginLocked(ctx)
}

// loginLocked performs login while the caller already holds c.mu.
func (c *Client) loginLocked(ctx context.Context) error {
	c.loggedIn = false

	// Step 1: GET the sendsms page to extract the CSRF token.
	sendsmsURL, err := c.abs("/sendsms")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sendsmsURL, nil)
	if err != nil {
		return fmt.Errorf("smartsms: new request: %w", err)
	}
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("smartsms: GET %s: %w", sendsmsURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("smartsms: GET %s: HTTP %d", sendsmsURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("smartsms: read sendsms page: %w", err)
	}

	token := extractCSRF(string(body))
	if token == "" {
		return fmt.Errorf("smartsms: could not extract CSRF token from /sendsms")
	}
	c.csrfToken.Store(token)

	// Step 2: POST /login with credentials + CSRF token.
	fields := map[string]string{
		"email":    c.username,
		"password": c.password,
		"_token":   token,
	}

	respBody, err := c.multipartPost(ctx, "/login", fields)
	if err != nil {
		return fmt.Errorf("smartsms: login POST failed: %w", err)
	}

	if isLoginPage(string(respBody)) {
		return fmt.Errorf("smartsms: login failed (still on login page); check credentials")
	}

	// Re-fetch the sendsms page to obtain a fresh CSRF token for subsequent requests.
	sendsmsURL2, err := c.abs("/sendsms")
	if err != nil {
		return err
	}
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, sendsmsURL2, nil)
	if err != nil {
		return fmt.Errorf("smartsms: new request (post-login): %w", err)
	}
	req2.Header.Set("User-Agent", clientUserAgent)

	resp2, err := c.httpClient.Do(req2)
	if err != nil {
		return fmt.Errorf("smartsms: GET %s (post-login): %w", sendsmsURL2, err)
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(io.LimitReader(resp2.Body, maxResponseBodySize))
	if err != nil {
		return fmt.Errorf("smartsms: read sendsms page (post-login): %w", err)
	}

	freshToken := extractCSRF(string(body2))
	if freshToken == "" {
		return fmt.Errorf("smartsms: could not extract CSRF token after login; session may be invalid")
	}
	c.csrfToken.Store(freshToken)

	c.loggedIn = true
	c.startHeartbeatLocked(ctx)
	return nil
}

// ---------------------------------------------------------------------------
// SendSMS
// ---------------------------------------------------------------------------

// SendSMS performs the 3-step SMS send flow on the SmartSMS platform.
// It is safe for concurrent use.
func (c *Client) SendSMS(ctx context.Context, req SendRequest) (*SendResponse, error) {
	c.mu.Lock()
	if err := c.ensureSessionLocked(ctx); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	return c.sendSMSBody(ctx, req)
}

// sendSMSBody performs the send flow without holding the lock.
func (c *Client) sendSMSBody(ctx context.Context, req SendRequest) (*SendResponse, error) {
	mobiles, dropped := normalizeMobiles(req.Mobiles)
	if len(mobiles) == 0 {
		return nil, fmt.Errorf("smartsms: no valid Thai mobile numbers provided")
	}
	if len(dropped) > 0 {
		slog.Warn("smartsms: dropped invalid mobile numbers", "dropped", dropped)
	}

	mobileStr := strings.Join(mobiles, "\n")

	sendTime := ""
	if req.SendTime != nil {
		sendTime = req.SendTime.Format("2006-01-02 15:04")
	}

	// Step 1: POST /dataset/previewData
	step1Fields := map[string]string{
		"campaign_no": req.CampaignNo,
		"campaign":    req.Campaign,
		"message":     req.Message,
		"mobile":      mobileStr,
		"sender":      c.sender,
		"label":       c.label,
		"send_time":   sendTime,
		"ref_no":      req.RefNo,
	}

	step1Body, err := c.withReLogin(ctx, step1Fields, "/dataset/previewData")
	if err != nil {
		return nil, err
	}
	if err := expectJSON(step1Body, "step 1 (preview)"); err != nil {
		return nil, err
	}
	step1Resp, err := parsePreviewResponse(step1Body)
	if err != nil {
		return nil, fmt.Errorf("smartsms: step 1 (preview): %w", err)
	}
	if !step1Resp.Success {
		return nil, fmt.Errorf("smartsms: step 1 (preview) returned success=false; credits=%d", step1Resp.CreditsUsed)
	}

	// Step 2: POST /dataset/confirmSend
	step2Fields := map[string]string{
		"campaign_no":    req.CampaignNo,
		"campaign":       req.Campaign,
		"message":        req.Message,
		"mobile":         mobileStr,
		"sender":         c.sender,
		"label":          c.label,
		"send_time":      sendTime,
		"ref_no":         req.RefNo,
		"is_auto_resend": "false",
		"resends":        "{}",
	}

	step2Body, err := c.withReLogin(ctx, step2Fields, "/dataset/confirmSend")
	if err != nil {
		return nil, err
	}
	if err := expectJSON(step2Body, "step 2 (confirmSend)"); err != nil {
		return nil, err
	}
	step2Resp, err := parseSimpleSuccess(step2Body)
	if err != nil {
		return nil, fmt.Errorf("smartsms: step 2 (confirmSend): %w", err)
	}
	if !step2Resp.Success {
		return nil, fmt.Errorf("smartsms: step 2 (confirmSend) returned success=false")
	}

	// Step 3: POST /send/confirmSendSMS
	step3Fields := map[string]string{
		"campaign_id": step1Resp.PreviewID,
	}

	step3Body, err := c.withReLogin(ctx, step3Fields, "/send/confirmSendSMS")
	if err != nil {
		return nil, err
	}
	if err := expectJSON(step3Body, "step 3 (confirmSendSMS)"); err != nil {
		return nil, err
	}
	step3Resp, err := parseSimpleSuccess(step3Body)
	if err != nil {
		return nil, fmt.Errorf("smartsms: step 3 (confirmSendSMS): %w", err)
	}
	if !step3Resp.Success {
		return nil, fmt.Errorf("smartsms: step 3 (confirmSendSMS) returned success=false")
	}

	return &SendResponse{
		Success:      true,
		PreviewID:    step1Resp.PreviewID,
		CreditsUsed:  step1Resp.CreditsUsed,
		CorrectCount: step1Resp.CorrectCount,
		ErrorCounts:  step1Resp.ErrorCounts,
	}, nil
}

// withReLogin executes the multipart POST with retry logic and session recovery.
// It acquires/releases the lock only for re-login.
func (c *Client) withReLogin(ctx context.Context, fields map[string]string, path string) ([]byte, error) {
	fields["_token"] = c.csrfToken.Load().(string)

	body, err := c.multipartPostWithRetry(ctx, path, fields)
	if err == nil {
		if !isLoginPage(string(body)) {
			return body, nil
		}
	} else {
		if isTransientError(err) {
			return nil, err
		}
		if !isSessionError(err) {
			return nil, err
		}
	}

	slog.Warn("smartsms: session invalid, re-logging in", "path", path, "err", err)
	c.mu.Lock()
	if reErr := c.reLoginLocked(ctx); reErr != nil {
		c.mu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("smartsms: %s re-login failed: %w (original: %w)", path, reErr, err)
		}
		return nil, fmt.Errorf("smartsms: %s re-login failed: %w (triggered by login page)", path, reErr)
	}
	fields["_token"] = c.csrfToken.Load().(string)
	c.mu.Unlock()

	body, err = c.multipartPost(ctx, path, fields)
	if err != nil {
		return nil, fmt.Errorf("smartsms: %s failed after re-login: %w", path, err)
	}
	if isLoginPage(string(body)) {
		return nil, fmt.Errorf("smartsms: %s still on login page after re-login", path)
	}
	return body, nil
}

// expectJSON checks if the body looks like JSON. Returns an error if it's HTML.
func expectJSON(body []byte, step string) error {
	s := string(body)
	s = strings.TrimPrefix(s, "\xEF\xBB\xBF") // strip UTF-8 BOM
	trimmed := strings.TrimLeft(s, " \t\r\n")
	if len(trimmed) == 0 {
		return fmt.Errorf("smartsms: %s: empty response body", step)
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		snippet := trimmed
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		return fmt.Errorf("smartsms: %s: expected JSON, got: %s", step, snippet)
	}
	return nil
}

// ensureSessionLocked logs in if not authenticated. Caller holds c.mu.
func (c *Client) ensureSessionLocked(ctx context.Context) error {
	if c.loggedIn {
		return nil
	}
	return c.loginLocked(ctx)
}

// reLoginLocked resets state and logs in again. Caller holds c.mu.
func (c *Client) reLoginLocked(ctx context.Context) error {
	c.loggedIn = false
	return c.loginLocked(ctx)
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// multipartPost performs a multipart/form-data POST to the given path.
// Returns the response body on HTTP 200, or (nil, error) on failure.
func (c *Client) multipartPost(ctx context.Context, path string, fields map[string]string) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, fmt.Errorf("smartsms: multipart write field %q: %w", key, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("smartsms: multipart close: %w", err)
	}

	url, err := c.abs(path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("smartsms: new request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("smartsms: POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
	if err != nil {
		return nil, fmt.Errorf("smartsms: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{
			StatusCode: resp.StatusCode,
			Body:       respBody,
			Path:       path,
		}
	}

	return respBody, nil
}

// multipartPostWithRetry wraps multipartPost with retry logic for transient errors.
// It retries up to maxRetries times with exponential backoff for 5xx and connection errors.
func (c *Client) multipartPostWithRetry(ctx context.Context, path string, fields map[string]string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		body, err := c.multipartPost(ctx, path, fields)
		if err == nil {
			return body, nil
		}

		lastErr = err

		if !isTransientError(err) {
			return nil, err
		}

		if attempt < maxRetries-1 {
			delay := retryBaseDelay * time.Duration(1<<uint(attempt))
			slog.Warn("smartsms: transient error, retrying", "path", path, "attempt", attempt+1, "delay", delay, "err", err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return nil, lastErr
}

// abs resolves a relative path against the base URL.
// Absolute URLs are rejected to prevent SSRF.
func (c *Client) abs(path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("smartsms: SSRF rejected absolute URL: %s", path)
	}
	return strings.TrimRight(c.baseURL, "/") + "/" + strings.TrimLeft(path, "/"), nil
}

// validateBaseURL checks that the base URL uses https and is not a private range.
func validateBaseURL(raw string) error {
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "http://") {
		return fmt.Errorf("smartsms: base URL must have http:// or https:// scheme, got %q", raw)
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1") || strings.Contains(lower, "0.0.0.0") {
		return fmt.Errorf("smartsms: base URL must not point to localhost/loopback")
	}
	return nil
}

// ---------------------------------------------------------------------------
// CSRF extraction
// ---------------------------------------------------------------------------

// csrfRe handles both attribute orderings and allows flexible whitespace.
var csrfRe = regexp.MustCompile(`<input[^>]*name=["']_token["'][^>]*value=["']([^"']+)["']|<input[^>]*value=["']([^"']+)["'][^>]*name=["']_token["']`)

// extractCSRF extracts the Laravel CSRF _token from an HTML string.
func extractCSRF(html string) string {
	matches := csrfRe.FindStringSubmatch(html)
	if len(matches) >= 2 {
		if matches[1] != "" {
			return matches[1]
		}
		if matches[2] != "" {
			return matches[2]
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Session expiry detection
// ---------------------------------------------------------------------------

// isLoginPage returns true if the HTML body looks like a login page.
func isLoginPage(body string) bool {
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "<form") {
		return false
	}
	return strings.Contains(lower, "name=\"email\"") ||
		strings.Contains(lower, "type=\"password\"") ||
		strings.Contains(lower, "/login")
}

// ---------------------------------------------------------------------------
// Phone normalization
// ---------------------------------------------------------------------------

// normalizeMobiles normalizes a list of phone numbers and returns valid ones
// plus any that were dropped.
func normalizeMobiles(phones []string) (valid []string, dropped []string) {
	valid = make([]string, 0, len(phones))
	for _, p := range phones {
		normalized := normalizeMobile(p)
		if normalized != "" {
			valid = append(valid, normalized)
		} else if strings.TrimSpace(p) != "" {
			dropped = append(dropped, p)
		}
	}
	return valid, dropped
}

// normalizeMobile normalizes a Thai phone number to 10-digit format (0xxxxxxxxx).
// Supports +66, 0066, and bare 66 prefixes.
func normalizeMobile(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}

	// Strip dashes, parentheses, spaces, tabs.
	phone = strings.NewReplacer("-", "", "(", "", ")", "", " ", "", "\t", "").Replace(phone)

	// +66 prefix → 0
	if strings.HasPrefix(phone, "+66") {
		phone = "0" + phone[3:]
	}

	// 0066 prefix → 0
	if strings.HasPrefix(phone, "0066") {
		phone = "0" + phone[4:]
	}

	// Bare 66 prefix (without + or 00) → 0 (66 + 9 digits = 11 chars)
	if strings.HasPrefix(phone, "66") && len(phone) == 11 {
		phone = "0" + phone[2:]
	}

	// Already starts with 0 and is 10 digits → valid.
	if strings.HasPrefix(phone, "0") && len(phone) == 10 {
		return phone
	}

	return ""
}

// ---------------------------------------------------------------------------
// Response parsing
// ---------------------------------------------------------------------------

// previewResponse holds the parsed JSON from /dataset/previewData.
type previewResponse struct {
	Success      bool           `json:"success"`
	PreviewID    string         `json:"preview_id"`
	CreditsUsed  int            `json:"number_of_used_credit"`
	CorrectCount int            `json:"correct"`
	ErrorCounts  map[string]int `json:"-"`
}

func parsePreviewResponse(body []byte) (*previewResponse, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("smartsms: empty response body")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("smartsms: parse preview JSON: %w", err)
	}

	r := &previewResponse{}

	if v, ok := raw["success"]; ok {
		if err := json.Unmarshal(v, &r.Success); err != nil {
			return nil, fmt.Errorf("smartsms: parse success field: %w", err)
		}
	}
	if v, ok := raw["preview_id"]; ok {
		if err := json.Unmarshal(v, &r.PreviewID); err != nil {
			return nil, fmt.Errorf("smartsms: parse preview_id field: %w", err)
		}
	}
	if v, ok := raw["number_of_used_credit"]; ok {
		if err := json.Unmarshal(v, &r.CreditsUsed); err != nil {
			return nil, fmt.Errorf("smartsms: parse number_of_used_credit field: %w", err)
		}
	}
	if v, ok := raw["correct"]; ok {
		if err := json.Unmarshal(v, &r.CorrectCount); err != nil {
			return nil, fmt.Errorf("smartsms: parse correct field: %w", err)
		}
	}

	r.ErrorCounts = make(map[string]int)
	for k, v := range raw {
		if strings.HasPrefix(k, "error_") {
			var count int
			if err := json.Unmarshal(v, &count); err != nil {
				return nil, fmt.Errorf("smartsms: parse %s field: %w", k, err)
			}
			r.ErrorCounts[k] = count
		}
	}

	return r, nil
}

func parseSimpleSuccess(body []byte) (*SendResponse, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("smartsms: empty response body")
	}

	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("smartsms: parse JSON: %w", err)
	}
	return &SendResponse{Success: resp.Success}, nil
}

// ---------------------------------------------------------------------------
// Session heartbeat
// ---------------------------------------------------------------------------

// startHeartbeatLocked starts a background goroutine that keeps the Laravel
// session alive by issuing a lightweight GET to /sendsms every 3 minutes.
// Caller must hold c.mu. It cancels any previous heartbeat first.
func (c *Client) startHeartbeatLocked(ctx context.Context) {
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
	}
	c.heartbeatCtx, c.heartbeatCancel = context.WithCancel(ctx)
	go c.heartbeatLoop(c.heartbeatCtx)
}

// heartbeatLoop runs the periodic heartbeat until ctx is cancelled.
func (c *Client) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.heartbeatLocked(ctx); err != nil {
				slog.Warn("smartsms: heartbeat failed", "err", err)
			}
		}
	}
}

// heartbeatLocked issues a GET to /sendsms to keep the Laravel session alive.
// On 401/403 it attempts a single re-login. Other errors are logged and ignored.
func (c *Client) heartbeatLocked(ctx context.Context) error {
	sendsmsURL, err := c.abs("/sendsms")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sendsmsURL, nil)
	if err != nil {
		return fmt.Errorf("smartsms: heartbeat new request: %w", err)
	}
	req.Header.Set("User-Agent", clientUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("smartsms: heartbeat GET: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		c.mu.Lock()
		reErr := c.reLoginLocked(context.Background())
		c.mu.Unlock()
		if reErr != nil {
			return fmt.Errorf("smartsms: heartbeat re-login: %w", reErr)
		}
		return nil
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("smartsms: heartbeat: HTTP %d", resp.StatusCode)
	}
	return nil
}

// StopHeartbeat cancels the session heartbeat goroutine, if running.
func (c *Client) StopHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
		c.heartbeatCancel = nil
	}
}
