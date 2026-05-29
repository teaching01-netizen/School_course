// Package crmclient provides an HTTP client for interacting with Sage CRM's
// eware.dll web interface. It supports session-based authentication via
// cookie jar and scraping of HTML forms to discover required fields, and
// can download XLSX exports of CRM reports.
package crmclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"
)

// Config holds the credentials and endpoint for the Sage CRM instance.
type Config struct {
	// BaseURL is the root of the CRM web app, e.g. "http://warwickins.sundaehost.com/crm"
	BaseURL string
	// Username is the CRM login username (e.g., "RDteam1")
	Username string
	// Password is the CRM login password
	Password string
	// RequestTimeout is the per-request timeout. Zero uses defaults (30s normal, 120s download).
	RequestTimeout time.Duration
	// DownloadTimeout is the timeout for XLSX download requests. Zero defaults to 120s.
	DownloadTimeout time.Duration
}

// Client is a stateful HTTP client for Sage CRM that manages sessions via
// a cookie jar and remembers the SID extracted after login.
type Client struct {
	httpClient      *http.Client
	baseURL         string
	username        string
	password        string
	sid             string
	requestTimeout  time.Duration
	downloadTimeout time.Duration
}

// New creates a new Sage CRM client. It does not connect or log in; call
// Login to establish a session.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("crmclient: BaseURL is required")
	}
	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("crmclient: Username and Password are required")
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, fmt.Errorf("crmclient: cookiejar: %w", err)
	}

	requestTimeout := cfg.RequestTimeout
	if requestTimeout == 0 {
		requestTimeout = 30 * time.Second
	}
	downloadTimeout := cfg.DownloadTimeout
	if downloadTimeout == 0 {
		downloadTimeout = 120 * time.Second
	}

	return &Client{
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: requestTimeout,
		},
		baseURL:         strings.TrimRight(cfg.BaseURL, "/"),
		username:        cfg.Username,
		password:        cfg.Password,
		requestTimeout:  requestTimeout,
		downloadTimeout: downloadTimeout,
	}, nil
}

// SID returns the current Sage CRM session ID extracted after login.
func (c *Client) SID() string {
	return c.sid
}

// LoggedIn returns true if a session ID has been extracted.
func (c *Client) LoggedIn() bool {
	return c.sid != ""
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

// Login authenticates with Sage CRM by:
//  1. GET /crm/eware.dll/go to obtain the login form and a temporary SID.
//  2. Parsing the HTML for hidden form inputs and the form action URL.
//  3. POSTing credentials together with the discovered hidden fields.
//  4. If the response shows "already logged on", re-submit the form
//     with LoginAnyway=T to terminate the old session.
//  5. Extracting the SID from the final response.
//
// After a successful login the client's cookie jar holds the session cookie
// and SID is available via SID().
func (c *Client) Login(ctx context.Context) error {
	c.sid = "" // reset on any login attempt

	// If the account is temporarily locked (often due to repeated attempts),
	// Sage CRM presents an intermediate page with a "Continue" action. We
	// automatically follow that flow and retry the login, waiting 20s
	// between attempts to give the lock time to clear.
	const maxLockRetries = 5
	lockRetryDelay := 20 * time.Second
	var lastErr error
	for attempt := 0; attempt <= maxLockRetries; attempt++ {
		if attempt > 0 {
			// Respect context cancellation while waiting.
			t := time.NewTimer(lockRetryDelay)
			select {
			case <-ctx.Done():
				t.Stop()
				if lastErr != nil {
					return fmt.Errorf("crmclient: login aborted: %w (after %d lock retries)", lastErr, attempt-1)
				}
				return fmt.Errorf("crmclient: login aborted: %w", ctx.Err())
			case <-t.C:
			}
		}

		if err := c.loginOnce(ctx); err != nil {
			lastErr = err
			if isAccountLockedError(err) && attempt < maxLockRetries {
				continue
			}
			return err
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("crmclient: login failed (unknown error)")
}

func (c *Client) loginOnce(ctx context.Context) error {
	goURL := c.abs("/eware.dll/go")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, goURL, nil)
	if err != nil {
		return fmt.Errorf("crmclient: new request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("crmclient: GET %s: %w", goURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("crmclient: read login page: %w", err)
	}

	// Parse the HTML to find form action and hidden inputs.
	formAction, hiddenFields, err := parseLoginForm(string(body))
	if err != nil {
		return fmt.Errorf("crmclient: parse login form: %w", err)
	}

	if err := c.submitLoginForm(ctx, formAction, hiddenFields, false); err != nil {
		return err
	}
	return nil
}

// submitLoginForm POSTs login credentials to the CRM. If reconfirm is true,
// the hidden fields already include LoginAnyway=T and the pre-filled
// EWARE_USERID/PASSWORD fields from the "already logged on" page.
func (c *Client) submitLoginForm(ctx context.Context, formAction string, hiddenFields formFields, reconfirm bool) error {
	formData := url.Values{}
	for k, v := range hiddenFields {
		formData.Set(k, v)
	}
	if !reconfirm {
		// First attempt: fill in credentials. Hidden fields from the
		// login page contain defaults for BrowserSupportsJavascript etc.
		formData.Set("EWARE_USERID", c.username)
		formData.Set("PASSWORD", c.password)
		// Sage CRM expects these flags even without a real browser.
		formData.Set("BrowserSupportsJavascript", "1")
		formData.Set("BrowserSupportsAJAX", "1")
	}
	// On reconfirm the hidden fields already contain LoginAnyway=T and
	// the pre-filled credentials — just submit as-is.

	loginURL := c.abs(formAction)
	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("crmclient: new login request: %w", err)
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", userAgent)
	// Prevent connection reuse which can confuse IIS-based Sage CRM.
	loginReq.Close = true

	loginResp, err := c.httpClient.Do(loginReq)
	if err != nil {
		return fmt.Errorf("crmclient: POST %s: %w", loginURL, err)
	}
	defer loginResp.Body.Close()

	loginBody, err := io.ReadAll(loginResp.Body)
	if err != nil {
		return fmt.Errorf("crmclient: read login response: %w", err)
	}
	bodyStr := string(loginBody)

	// Sage CRM can show an account lock/interstitial page with a "Continue"
	// form; follow it. If clicking Continue returns the dashboard with
	// a SID, login is complete. Otherwise signal a retry.
	if isAccountLockedPage(bodyStr) {
		if err := c.followContinue(ctx, bodyStr); err != nil {
			return err
		}
		// If followContinue extracted a SID from the Continue response,
		// the login completed successfully — no retry needed.
		if c.sid != "" {
			return nil
		}
		return &accountLockedErr{msg: "account locked (continue followed); retrying login"}
	}

	// Check for "already logged on" — if so, re-submit with LoginAnyway=T.
	alreadyLoggedOn := strings.Contains(bodyStr, "already logged on")
	if alreadyLoggedOn {
		if reconfirm {
			// We already tried with LoginAnyway=T and it still shows
			// the conflict page — something is wrong.
			return fmt.Errorf("crmclient: login failed: session conflict persists after re-login attempt")
		}
		// Parse the hidden fields from the conflict page and re-submit.
		_, conflictFields, err := parseLoginForm(bodyStr)
		if err != nil {
			return fmt.Errorf("crmclient: parse conflict form: %w", err)
		}
		return c.submitLoginForm(ctx, formAction, conflictFields, true)
	}

	// Extract SID from the response body or any redirect URL.
	sid := extractSID(bodyStr)
	if sid == "" && loginResp.Request != nil {
		sid = extractSID(loginResp.Request.URL.String())
	}
	if sid == "" {
		if loc := loginResp.Header.Get("Location"); loc != "" {
			sid = extractSID(loc)
		}
	}
	if sid == "" {
		return fmt.Errorf("crmclient: login failed (no SID in response); check credentials or CRM availability")
	}
	c.sid = sid
	return nil
}

// ---------------------------------------------------------------------------
// Account lock / Continue interstitial handling
// ---------------------------------------------------------------------------

type accountLockedErr struct {
	msg string
}

func (e *accountLockedErr) Error() string { return e.msg }

func isAccountLockedError(err error) bool {
	_, ok := err.(*accountLockedErr)
	return ok
}

// isAccountLockedPage detects a Sage CRM interstitial that requires clicking
// "Continue" (e.g. account temporarily locked, too many attempts, or
// another user session conflict).
//
// Detection is based on the presence of an HTML form that lacks the
// EWARE_USERID field — login pages and "already logged on" pages always
// carry that field, so any other form is an interstitial that should be
// submitted to advance.
func isAccountLockedPage(body string) bool {
	lower := strings.ToLower(body)

	// If the page contains EWARE_USERID, it's a login or "already logged
	// on" page — not a lock interstitial.
	if strings.Contains(lower, "eware_userid") {
		return false
	}

	// Check if there's a form we can submit to advance.
	if _, _, err := parseForm(body, true /* includeSubmit */); err == nil {
		return true
	}

	return false
}

// followContinue attempts to submit the first form on the page, including
// submit/button fields, emulating clicking "Continue".
// followContinue submits the first form on the page (the "Continue" action
// on a lock/interstitial page) and reads the response. Sage CRM often
// returns the main dashboard with a valid SID after clicking Continue,
// completing the login. If a SID is found, it is set on the client.
func (c *Client) followContinue(ctx context.Context, htmlBody string) error {
	action, fields, err := parseForm(htmlBody, true /* includeSubmit */)
	if err != nil {
		return fmt.Errorf("crmclient: parse continue form: %w", err)
	}
	formData := url.Values{}
	for k, v := range fields {
		formData.Set(k, v)
	}

	continueURL := c.abs(action)
	continueReq, err := http.NewRequestWithContext(ctx, http.MethodPost, continueURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("crmclient: new continue request: %w", err)
	}
	continueReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	continueReq.Header.Set("User-Agent", userAgent)
	continueReq.Close = true

	resp, err := c.httpClient.Do(continueReq)
	if err != nil {
		return fmt.Errorf("crmclient: POST continue %s: %w", continueURL, err)
	}
	defer resp.Body.Close()

	// Read the full response — Sage CRM may return the main dashboard
	// page (with a valid SID) after clicking Continue, completing the
	// login. If we find a SID, set it on the client as a successful login.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("crmclient: read continue response: %w", err)
	}
	bodyStr := string(body)

	sid := extractSID(bodyStr)
	if sid == "" && resp.Request != nil {
		sid = extractSID(resp.Request.URL.String())
	}
	if sid == "" {
		if loc := resp.Header.Get("Location"); loc != "" {
			sid = extractSID(loc)
		}
	}
	if sid != "" {
		c.sid = sid
	}

	return nil
}

// ---------------------------------------------------------------------------
// Report Download
// ---------------------------------------------------------------------------

// DownloadXLSX logs in if needed, then downloads an XLSX export from Sage CRM.
//
// The flow:
//  1. Login (if not already logged in).
//  2. GET the report criteria action (Act=1410&Mode=4) to load the saved
//     filter configuration.
//  3. GET the export endpoint (Act=1411&Destination=Formats&ReportFormat=XLSX)
//     to receive the XLSX bytes.
//
// If the download returns HTML (indicating a stale session), the client
// re-logs in once and retries.
//
// criteria holds URL-encoded query parameters that define the report, such
// as Key0=41&Key4=35&Key41=864&Key62=1012 for the "Total Students Each Cycle"
// report.
func (c *Client) DownloadXLSX(ctx context.Context, criteria url.Values) ([]byte, error) {
	if !c.LoggedIn() {
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
	}

	data, err := c.downloadXLSX(ctx, criteria)
	if err != nil && isSessionError(err) {
		// Session expired — re-login and retry once.
		if loginErr := c.Login(ctx); loginErr != nil {
			return nil, fmt.Errorf("crmclient: re-login failed after session expiry: %w", loginErr)
		}
		data, err = c.downloadXLSX(ctx, criteria)
	}
	return data, err
}

// downloadXLSX performs the actual download without session retry logic.
func (c *Client) downloadXLSX(ctx context.Context, criteria url.Values) ([]byte, error) {
	// Build the download-specific URL including any report filter criteria
	// (e.g. Key0=41&Key4=35&Key41=864&Key62=1012 for the
	// "Total Students Each Cycle" report).
	downloadURL := c.ewareURL("Do", url.Values{
		"Act":          {"1411"},
		"Destination":  {"Formats"},
		"ReportFormat": {"XLSX"},
	}, criteria)

	// Temporarily switch to download-specific timeout to avoid data
	// races from copying the http.Client struct (which holds the cookie
	// jar mutex inside).
	origTimeout := c.httpClient.Timeout
	c.httpClient.Timeout = c.downloadTimeout
	defer func() { c.httpClient.Timeout = origTimeout }()

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("crmclient: new download request: %w", err)
	}
	dlReq.Header.Set("User-Agent", userAgent)
	dlReq.Close = true

	dlResp, err := c.httpClient.Do(dlReq)
	if err != nil {
		return nil, fmt.Errorf("crmclient: GET download: %w", err)
	}
	defer dlResp.Body.Close()

	// Check if the response is HTML (indicating auth failure or session expiry).
	contentType := dlResp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		sample, _ := io.ReadAll(io.LimitReader(dlResp.Body, 2048))
		return nil, &sessionErr{msg: fmt.Sprintf("download returned HTML (auth failure): %s", trimSample(string(sample)))}
	}

	xlsxData, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return nil, fmt.Errorf("crmclient: read download body: %w", err)
	}

	return xlsxData, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// abs resolves a path against the CRM base URL.
//
// If ref is an absolute path (starts with "/") and already includes the
// base URL path prefix (e.g. "/crm/eware.dll/go" when base is "/crm"),
// only the scheme+host is prepended. Otherwise the full base URL is used.
func (c *Client) abs(ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}

	u, _ := url.Parse(c.baseURL)
	basePath := strings.TrimRight(u.Path, "/")

	if strings.HasPrefix(ref, "/") {
		if basePath != "" && strings.HasPrefix(ref, basePath) {
			// ref already includes the app root path, e.g. "/crm/eware.dll/go"
			return u.Scheme + "://" + u.Host + ref
		}
		// ref is an absolute path without app root, e.g. "/eware.dll/Do"
		return u.Scheme + "://" + u.Host + basePath + ref
	}

	// Relative path — just join.
	return strings.TrimRight(c.baseURL, "/") + "/" + strings.TrimLeft(ref, "/")
}

// ewareURL builds a full CRM eware.dll URL with the current SID and the given
// mandatory and optional query parameters.
//
// mandatory holds params that must be set (e.g. Act, Mode).
// optional holds extra user-supplied params (e.g. Key0, Key4 values) which
// are merged into the query string after the mandatory params.
func (c *Client) ewareURL(path string, mandatory, optional url.Values) string {
	q := url.Values{}
	q.Set("SID", c.sid)
	for k, vals := range mandatory {
		for _, v := range vals {
			q.Add(k, v)
		}
	}
	for k, vals := range optional {
		for _, v := range vals {
			q.Add(k, v)
		}
	}
	return c.abs("/eware.dll/"+path) + "?" + q.Encode()
}

// ---------------------------------------------------------------------------
// HTML Form Parsing
// ---------------------------------------------------------------------------

// formFields is a simple key-value map of HTML input fields.
type formFields map[string]string

// parseLoginForm extracts the form action URL and all input fields from a
// Sage CRM login page HTML.
func parseLoginForm(htmlContent string) (action string, fields formFields, err error) {
	return parseForm(htmlContent, false /* includeSubmit */)
}

func parseForm(htmlContent string, includeSubmit bool) (action string, fields formFields, err error) {
	doc, parseErr := html.Parse(strings.NewReader(htmlContent))
	if parseErr != nil {
		return "", nil, fmt.Errorf("parse HTML: %w", parseErr)
	}

	fields = make(formFields)
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			for _, attr := range n.Attr {
				if attr.Key == "action" {
					action = attr.Val
				}
			}
			scanInputs(n, fields, includeSubmit)
			return // only scan the first form
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			f(child)
		}
	}
	f(doc)

	if action == "" {
		return "", nil, fmt.Errorf("no form found in page")
	}
	return action, fields, nil
}

// scanInputs recursively scans HTML nodes for <input>, <select>, and (optionally)
// <button> elements and stores their name/value pairs into fields.
func scanInputs(n *html.Node, fields formFields, includeSubmit bool) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "input":
			var name, value, typ string
			for _, attr := range n.Attr {
				switch attr.Key {
				case "name":
					name = attr.Val
				case "value":
					value = attr.Val
				case "type":
					typ = attr.Val
				}
			}
			if name == "" {
				break
			}
			if (typ == "submit" || typ == "button") && !includeSubmit {
				break
			}
			if typ == "submit" || typ == "button" || typ == "" {
				// keep value as-is (often empty for submit)
			}
			if name != "" {
				fields[name] = value
			}
		case "select":
			// Capture the select name — the actual value will be set
			// by the selected <option>, but for login forms the hidden
			// defaults are sufficient.
			var name string
			for _, attr := range n.Attr {
				if attr.Key == "name" {
					name = attr.Val
				}
			}
			if name != "" {
				// Pick the first option value (the default selected).
				if opt := findSelectedOption(n); opt != "" {
					if _, exists := fields[name]; !exists {
						fields[name] = opt
					}
				}
			}
		case "button":
			if !includeSubmit {
				break
			}
			var name, value string
			for _, attr := range n.Attr {
				switch attr.Key {
				case "name":
					name = attr.Val
				case "value":
					value = attr.Val
				}
			}
			if name != "" {
				fields[name] = value
			}
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		scanInputs(child, fields, includeSubmit)
	}
}

// findSelectedOption returns the value of the first <option> inside a <select>
// that has the "selected" attribute, or the first option if none is selected.
func findSelectedOption(n *html.Node) string {
	var firstValue string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "option" {
			var value, val string
			selected := false
			for _, attr := range child.Attr {
				switch attr.Key {
				case "value":
					val = attr.Val
				case "selected":
					selected = true
				}
			}
			if firstValue == "" {
				firstValue = val
			}
			if selected {
				value = val
			}
			if value != "" {
				return value
			}
		}
	}
	return firstValue
}

// userAgent is a standard browser User-Agent header value required by
// Sage CRM's IIS-based backend to avoid connection drops.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

var (
	sidRe    = regexp.MustCompile(`[?&]SID=(\d+)`)
	sidReAny = regexp.MustCompile(`SID=(\d+)`)
)

// extractSID looks for a SID=... parameter in URLs embedded in the given
// text (HTML or plain text) and returns the first occurrence found.
func extractSID(text string) string {
	matches := sidRe.FindStringSubmatch(text)
	if len(matches) >= 2 {
		return matches[1]
	}
	// Broader fallback.
	matches2 := sidReAny.FindStringSubmatch(text)
	if len(matches2) >= 2 {
		return matches2[1]
	}
	return ""
}

// trimSample truncates a string for error messages.
func trimSample(s string) string {
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

// ---------------------------------------------------------------------------
// Session error type
// ---------------------------------------------------------------------------

// sessionErr is returned when the CRM download response is HTML (indicating
// a stale or invalid session).
type sessionErr struct {
	msg string
}

func (e *sessionErr) Error() string { return e.msg }

// isSessionError returns true if err indicates a stale CRM session.
func isSessionError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*sessionErr)
	return ok
}
