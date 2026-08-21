package legacysync

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	sourceclient "warwick-institute/internal/legacysync/client"
)

// cachedFormTTL bounds how long a parse d search form (antiforgery token) is
// reused before the page is read again. Tokens are session-stable, so the
// rate of page reads is tiny; the TTL only caps the damage if a site ever
// rotated tokens mid-session. On re-login the generation check invalidates
// the cache immediately, and a failed search also forces a fresh read.
const cachedFormTTL = 5 * time.Minute

type Client struct {
	source     *sourceclient.Client
	httpClient *http.Client

	// Search-form caches. Each search/archive POST needs the page's hidden
	// antiforgery token; the token only changes on re-login, so caching it
	// (guarded by the auth generation) turns N lookups into 1 page read + N
	// searches instead of 2N requests. See cachedFormTTL.
	formMu          sync.Mutex
	studentsForm    *studentsSearchForm
	studentsFormAt  time.Time
	studentsFormGen uint64
	courseForm      *courseSearchForm
	courseFormAt    time.Time
	courseFormGen   uint64
}

// httpTimeoutFromEnv returns the per-request budget for the legacy site.
// The legacy site can be very slow (the login redirect chain alone can take
// tens of seconds), so this covers the full exchange including redirects and
// body download. Override with LEGACY_SYNC_HTTP_TIMEOUT (Go duration string).
func httpTimeoutFromEnv() time.Duration {
	if raw := os.Getenv("LEGACY_SYNC_HTTP_TIMEOUT"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 120 * time.Second
}

// maxConcurrentFromEnv returns how many requests may be in flight against
// the legacy site at once. The default (32) gives a large speedup over the
// historical 8 without hammering the site; raise or lower with
// LEGACY_SYNC_MAX_CONCURRENT.
func maxConcurrentFromEnv() int {
	raw := os.Getenv("LEGACY_SYNC_MAX_CONCURRENT")
	if raw == "" {
		return 32
	}
	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
		return parsed
	}
	return 32
}

// maxRequestsPerMinuteFromEnv returns the rolling per-minute cap on requests
// issued to the legacy site, enforced by the client's token-bucket budget.
// The default (720, ~12 requests/sec) keeps bursts bounded so egress stays
// steady; raise or lower with LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE.
func maxRequestsPerMinuteFromEnv() int {
	raw := os.Getenv("LEGACY_SYNC_MAX_REQUESTS_PER_MINUTE")
	if raw == "" {
		return 720
	}
	if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
		return parsed
	}
	return 720
}

// maxEgressBytesPerMinuteFromEnv returns the rolling per-minute cap on bytes
// downloaded from the legacy site, enforced by the client's token-bucket
// budget. The default (200 MiB) bounds the provider's egress bill while still
// allowing full crawls; override with LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE
// (byte counts only).
func maxEgressBytesPerMinuteFromEnv() int64 {
	raw := os.Getenv("LEGACY_SYNC_MAX_EGRESS_BYTES_PER_MINUTE")
	if raw == "" {
		return 200 << 20
	}
	if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed >= 0 {
		return parsed
	}
	return 200 << 20
}

// minRequestIntervalFromEnv returns the global minimum spacing between
// requests to the legacy site ("politeness" pacing). The default is 0 —
// disabled — so the concurrency semaphore and the per-minute budget govern
// throughput instead of a client-wide slot lock; override with
// LEGACY_SYNC_MIN_REQUEST_INTERVAL (Go duration, e.g. "500ms"), or set it to
// "0" to keep pacing disabled explicitly.
func minRequestIntervalFromEnv() time.Duration {
	raw := os.Getenv("LEGACY_SYNC_MIN_REQUEST_INTERVAL")
	if raw == "" {
		return 0
	}
	if parsed, err := time.ParseDuration(raw); err == nil && parsed >= 0 {
		return parsed
	}
	return 0
}

// ClientOption configures the legacy site client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	maxBodyBytes int64
}

// WithMaxBodyBytes raises the per-response body cap beyond the default
// 2 MiB. The archived course listing on the old site is several MB.
func WithMaxBodyBytes(n int64) ClientOption {
	return func(o *clientOptions) {
		if n > 16<<20 {
			n = 16 << 20
		}
		o.maxBodyBytes = n
	}
}

func NewClient(baseURL, username, password string, opts ...ClientOption) (*Client, error) {
	options := clientOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	maxBodyBytes := options.maxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = 2 << 20
	}
	if maxBodyBytes > 16<<20 {
		maxBodyBytes = 16 << 20
	}
	maxConcurrent := maxConcurrentFromEnv()
	if maxConcurrent < 1 {
		maxConcurrent = 32
	}
	if maxConcurrent > 128 {
		maxConcurrent = 128
	}
	httpClient := &http.Client{Timeout: httpTimeoutFromEnv(), Transport: httpTransportForConcurrency(maxConcurrent)}
	source, err := sourceclient.New(sourceclient.Config{
		BaseURL:                 baseURL,
		Username:                username,
		Password:                password,
		HTTPClient:              httpClient,
		MaxBodyBytes:            maxBodyBytes,
		MaxConcurrent:           maxConcurrent,
		MinRequestInterval:      minRequestIntervalFromEnv(),
		MaxRequestsPerMinute:    maxRequestsPerMinuteFromEnv(),
		MaxEgressBytesPerMinute: maxEgressBytesPerMinuteFromEnv(),
	})
	if err != nil {
		return nil, err
	}
	return &Client{source: source, httpClient: httpClient}, nil
}

// httpTransportForConcurrency returns a transport tuned for fan-out scraping:
// the stdlib default keeps only two idle keep-alive connections per host, so
// under the parallel load this client generates most requests would pay a
// fresh TCP+TLS handshake. Idle capacity is raised to the concurrency cap;
// everything else (HTTP/2, TLS, timeouts) is the stdlib default.
func httpTransportForConcurrency(maxConcurrent int) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	idle := maxConcurrent
	if idle < 2 {
		idle = 2
	}
	transport.MaxIdleConns = idle
	transport.MaxIdleConnsPerHost = idle
	return transport
}

func (c *Client) Login() error {
	return c.source.Login(context.Background())
}

// MaxConcurrent returns the client's in-flight request cap — the same value
// the low-level source client enforces. The legacy-sync worker sizes its
// parallel profile lookups from this so no caller queues more than the
// client can carry.
func (c *Client) MaxConcurrent() int { return c.source.MaxConcurrent() }

func (c *Client) CircuitState() (bool, time.Time) { return c.source.CircuitState() }

func (c *Client) EgressStats() (int, int64, time.Time) { return c.source.EgressStats() }

func (c *Client) BudgetExceeded() bool { return c.source.BudgetExceeded() }

func (c *Client) FetchSchedulePage(legacyCourseID string) (string, error) {
	return c.FetchSchedulePageContext(context.Background(), legacyCourseID)
}

func (c *Client) FetchSchedulePageContext(ctx context.Context, legacyCourseID string) (string, error) {
	response, err := c.source.Do(ctx, sourceclient.Request{
		Method: http.MethodGet,
		Path:   "/Admin/Courses/Detail",
		Query:  url.Values{"id": {legacyCourseID}},
	})
	if err != nil {
		return "", fmt.Errorf("fetch schedule page: %w", err)
	}
	return string(response.Body), nil
}

// FetchCourseListPageContext fetches the plain /Admin/Courses listing
// (active and draft courses). It never submits the search form: the
// search POST returns the archived-only list, and the plain GET returns
// the live courses.
func (c *Client) FetchCourseListPageContext(ctx context.Context) (string, error) {
	response, err := c.source.Do(ctx, sourceclient.Request{Method: http.MethodGet, Path: "/Admin/Courses"})
	if err != nil {
		return "", fmt.Errorf("fetch course list page: %w", err)
	}
	return string(response.Body), nil
}

// FetchStudentsPageContext fetches the /Admin/Students listing: the old
// site's student directory (wcode, name, nickname, school, level, year,
// phone). The page requires an authenticated session; the client re-authenticates
// automatically when the session has expired.
func (c *Client) FetchStudentsPageContext(ctx context.Context) (string, error) {
	response, err := c.source.Do(ctx, sourceclient.Request{Method: http.MethodGet, Path: "/Admin/Students"})
	if err != nil {
		return "", fmt.Errorf("fetch students page: %w", err)
	}
	return string(response.Body), nil
}

// SearchStudentsPageContext submits the /Admin/Students search form with the
// given SearchText (empty string lists every student). The plain page renders
// an empty table until the search handler runs, so the roster is always read
// through this request. A missing search form or antiforgery token is an
// error, never a silent fallback: the student directory must not vanish
// unnoticed.
//
// The search form (its antiforgery token) is session-stable and cached, so
// sequential lookups cost one POST each instead of a page read + POST. A
// failed search drops the cache and retries once with a freshly read form,
// so a rotated token heals without losing students.
func (c *Client) SearchStudentsPageContext(ctx context.Context, searchText string) (string, error) {
	form, err := c.loadStudentsForm(ctx)
	if err != nil {
		return "", err
	}
	posted, err := c.submitStudentsSearch(ctx, form.token, searchText)
	if err != nil {
		c.dropStudentsForm()
		form, refreshErr := c.loadStudentsForm(ctx)
		if refreshErr != nil {
			return "", fmt.Errorf("search students: %w", err)
		}
		posted, err = c.submitStudentsSearch(ctx, form.token, searchText)
		if err != nil {
			return "", fmt.Errorf("search students: %w", err)
		}
	}
	return string(posted.Body), nil
}

// loadStudentsForm returns the cached students search form, re-reading the
// page only when the cache is empty, older than cachedFormTTL, or from an
// older auth generation (a re-login re-issues the antiforgery cookie, which
// invalidates every previously parsed token).
func (c *Client) loadStudentsForm(ctx context.Context) (*studentsSearchForm, error) {
	c.formMu.Lock()
	defer c.formMu.Unlock()
	if c.studentsForm != nil && c.studentsFormGen == c.source.AuthGeneration() && time.Since(c.studentsFormAt) < cachedFormTTL {
		return c.studentsForm, nil
	}
	page, err := c.FetchStudentsPageContext(ctx)
	if err != nil {
		c.studentsForm = nil
		return nil, err
	}
	form, err := parseStudentsSearchForm(page)
	if err != nil {
		c.studentsForm = nil
		return nil, fmt.Errorf("search students: %w", err)
	}
	c.studentsForm = form
	c.studentsFormGen = c.source.AuthGeneration()
	c.studentsFormAt = time.Now()
	return form, nil
}

func (c *Client) dropStudentsForm() {
	c.formMu.Lock()
	c.studentsForm = nil
	c.formMu.Unlock()
}

func (c *Client) submitStudentsSearch(ctx context.Context, token, searchText string) (sourceclient.Response, error) {
	return c.source.Do(ctx, sourceclient.Request{
		Method: http.MethodPost,
		Path:   "/Admin/Students",
		Query:  url.Values{"handler": {"search"}},
		Form: url.Values{
			"SearchText":                 {searchText},
			"__RequestVerificationToken": {token},
		},
	})
}

// FetchArchivedCourseListPageContext fetches the archived-course listing.
// The old site hides archived courses unless its search form is submitted
// with IsArchive=true; that POST returns archived courses only, so callers
// merge this page with the plain listing. A missing search form or
// antiforgery token is an error, never a silent fallback: the archived
// list must not vanish unnoticed.
//
// Like the students search, the course search form is cached per session:
// the first call reads the list page for its token, later calls POST
// directly. A failed POST drops the cache and retries once with a fresh
// form.
func (c *Client) FetchArchivedCourseListPageContext(ctx context.Context) (string, error) {
	form, err := c.loadCourseForm(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch archived course list: %w", err)
	}
	posted, err := c.submitCourseSearch(ctx, form)
	if err != nil {
		c.dropCourseForm()
		form, refreshErr := c.loadCourseForm(ctx)
		if refreshErr != nil {
			return "", fmt.Errorf("fetch archived course list: %w", err)
		}
		posted, err = c.submitCourseSearch(ctx, form)
		if err != nil {
			return "", fmt.Errorf("fetch archived course list: %w", err)
		}
	}
	return string(posted.Body), nil
}

// loadCourseForm returns the cached course search form, re-reading the plain
// course list page for its token exactly like FetchArchivedCourseListPageContext
// historically did — the page read now happens only when the cache is cold,
// stale, or from a previous auth generation.
func (c *Client) loadCourseForm(ctx context.Context) (*courseSearchForm, error) {
	c.formMu.Lock()
	defer c.formMu.Unlock()
	if c.courseForm != nil && c.courseFormGen == c.source.AuthGeneration() && time.Since(c.courseFormAt) < cachedFormTTL {
		return c.courseForm, nil
	}
	page, err := c.FetchCourseListPageContext(ctx)
	if err != nil {
		c.courseForm = nil
		return nil, err
	}
	form, err := parseCourseSearchForm(page)
	if err != nil {
		c.courseForm = nil
		return nil, fmt.Errorf("fetch archived course list: %w", err)
	}
	c.courseForm = form
	c.courseFormGen = c.source.AuthGeneration()
	c.courseFormAt = time.Now()
	return form, nil
}

func (c *Client) dropCourseForm() {
	c.formMu.Lock()
	c.courseForm = nil
	c.formMu.Unlock()
}

// FetchArchivedWithPlainPage submits the archived search using a token derived from plainPage HTML without an extra GET.
func (c *Client) FetchArchivedWithPlainPage(ctx context.Context, plainPage string) (string, error) {
	form, err := parseCourseSearchForm(plainPage)
	if err != nil {
		return "", err
	}
	resp, err := c.submitCourseSearch(ctx, form)
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

func (c *Client) submitCourseSearch(ctx context.Context, form *courseSearchForm) (sourceclient.Response, error) {
	return c.source.Do(ctx, sourceclient.Request{
		Method: http.MethodPost,
		Path:   "/Admin/Courses",
		Query:  url.Values{"handler": {"search"}},
		Form: url.Values{
			"SearchText":                 {form.searchText},
			"IsArchive":                  {"true"},
			"__RequestVerificationToken": {form.token},
		},
	})
}
