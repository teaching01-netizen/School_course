package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	sourceclient "warwick-institute/internal/legacysync/client"
	"warwick-institute/internal/legacysync/normalize"
	"warwick-institute/internal/legacysync/parser"
)

// wcodeLookupRe matches a wcode the old site would print in its directory:
// the "W" prefix followed by digits (e.g. "W250025"). CRM-imported codes
// that do not match (free text, placeholders) are not valid directory keys
// and are skipped.
var wcodeLookupRe = regexp.MustCompile(`^W\d+$`)

// isSystemicProfileError classifies a profile-lookup failure that will not
// heal within this run: the source is rate limiting us, we tripped our own
// egress budget or circuit breaker, or the session can no longer authenticate.
// Per-wcode search/parse failures (including 4xx and transient source 5xx)
// keep their skip-and-retry-next-run behavior; a systemic failure instead
// aborts the whole profile phase so the reconcile is marked failed and the
// job retried with backoff instead of silently hammering a throttled source
// with a green run record.
func isSystemicProfileError(err error) bool {
	return errors.Is(err, sourceclient.ErrRateLimited) ||
		errors.Is(err, sourceclient.ErrEgressBudgetExceeded) ||
		errors.Is(err, sourceclient.ErrCircuitOpen) ||
		errors.Is(err, sourceclient.ErrAuthentication)
}

type StudentProfileProgress struct {
	CurrentWCode  string
	Processed     int
	Total         int
	ProfilesFound int
	Failures      int
}

// syncStudentProfiles observes the old site's /Admin/Students directory for
// every wcode the system knows and returns the matched directory profiles.
//
// The old site's students page only returns up to 50 rows per search, so a
// bulk fetch would silently truncate the directory. Exact wcode search text
// returns exactly one row, so each wcode is looked up individually — that is
// the only complete enumeration the page supports. A wcode the directory does
// not know (deleted or never imported upstream) simply yields no profile.
// Search/parse failures are logged and skipped per wcode so one flaky lookup
// never aborts the whole reconcile; the wcode is simply retried next run.
// Systemic failures — rate limiting, egress budget, open circuit, dead
// session — abort the phase instead (see isSystemicProfileError), so the
// reconcile is marked failed and retried with backoff rather than hammering
// a throttled source while reporting success.
//
// Lookups run through a bounded worker pool sized by the client's in-flight
// request cap (studentProfileWorkers) because per-wcode round trips dominate
// reconcile wall time; the client's own semaphore stays the hard ceiling, so
// the pool only keeps the HTTP pipeline full. Results are returned sorted by
// wcode so the batch profile apply sees a stable order regardless of
// completion order. When a progress callback is supplied, a single
// aggregator goroutine invokes it in completion order with monotonic
// processed/failure counts — the callback stays strictly sequential even
// though the lookups are not, and an error from it aborts the remaining
// lookups and fails the reconcile exactly like the sequential path did.
func (s *courseSyncer) syncStudentProfiles(ctx context.Context, progressCallbacks ...func(StudentProfileProgress) error) ([]normalize.LegacyStudent, error) {
	var progress func(StudentProfileProgress) error
	if len(progressCallbacks) > 0 {
		progress = progressCallbacks[0]
	}
	wcodes, err := s.listStudentWcodes(ctx)
	if err != nil {
		return nil, err
	}
	workers := s.studentProfileWorkers
	if workers < 1 {
		workers = 1
	}
	if len(wcodes) < workers {
		workers = len(wcodes)
	}
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()

	type lookupResult struct {
		wcode   string
		student *normalize.LegacyStudent // nil when the lookup failed or nothing matched
		failed  bool
	}
	var (
		firstErrMu sync.Mutex
		firstErr   error
	)
	jobs := make(chan string)
	results := make(chan lookupResult)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for wcode := range jobs {
				if err := workCtx.Err(); err != nil {
					return
				}
				page, err := s.client.SearchStudentsPageContext(workCtx, wcode)
				if err != nil {
					if isSystemicProfileError(err) {
						firstErrMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						firstErrMu.Unlock()
						cancelWork()
						results <- lookupResult{wcode: wcode, failed: true}
						return
					}
					s.log.Warn("legacy student profile lookup failed (skipping)", "wcode", wcode, "error", err)
					results <- lookupResult{wcode: wcode, failed: true}
					continue
				}
				result, err := parser.ParseStudentsPage(page)
				if err != nil {
					s.log.Warn("legacy student profile page unparsable (skipping)", "wcode", wcode, "error", err)
					results <- lookupResult{wcode: wcode, failed: true}
					continue
				}
				var matched *normalize.LegacyStudent
				for i := range result.Students {
					if strings.EqualFold(result.Students[i].WCode, wcode) {
						matched = &result.Students[i]
						break
					}
				}
				results <- lookupResult{wcode: wcode, student: matched}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, wcode := range wcodes {
			select {
			case jobs <- wcode:
			case <-workCtx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	var profiles []normalize.LegacyStudent
	processed, failures := 0, 0
	for res := range results {
		processed++
		if res.failed {
			failures++
		} else if res.student != nil {
			profiles = append(profiles, *res.student)
		}
		if progress != nil {
			if err := progress(StudentProfileProgress{
				CurrentWCode:  res.wcode,
				Processed:     processed,
				Total:         len(wcodes),
				ProfilesFound: len(profiles),
				Failures:      failures,
			}); err != nil {
				cancelWork()
				for range results { // keep workers unblocked while they wind down
				}
				return nil, err
			}
		}
	}
	firstErrMu.Lock()
	systemicErr := firstErr
	firstErrMu.Unlock()
	if systemicErr != nil {
		return nil, systemicErr
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].WCode < profiles[j].WCode })
	return profiles, nil
}

// listStudentWcodes returns the distinct wcodes the system knows that look
// like old-site directory keys. It reads the students table directly (not
// just legacy-managed refs) so CRM-imported students and manually created
// ones get their directory fields filled in too — the fill-in-if-empty upsert
// never overwrites what CRM or a human already entered.
func (s *courseSyncer) listStudentWcodes(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT wcode FROM students WHERE btrim(wcode) <> '' ORDER BY wcode`)
	if err != nil {
		return nil, fmt.Errorf("list student wcodes: %w", err)
	}
	defer rows.Close()
	var wcodes []string
	for rows.Next() {
		var wcode string
		if err := rows.Scan(&wcode); err != nil {
			return nil, fmt.Errorf("scan student wcode: %w", err)
		}
		if wcode := strings.ToUpper(strings.TrimSpace(wcode)); wcodeLookupRe.MatchString(wcode) {
			wcodes = append(wcodes, wcode)
		}
	}
	return wcodes, rows.Err()
}
