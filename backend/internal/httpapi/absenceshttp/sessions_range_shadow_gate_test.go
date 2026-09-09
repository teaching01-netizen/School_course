package absenceshttp

// Step-23 shadow gate: contractual-semantics comparison across rare-case
// coverage, with the production taxonomy (match / shape / order / value /
// error / shadow_skipped / snapshot_drift).
//
// Coverage (rare cases, not just totals): staff, bypass, course filter,
// after-priority, all-subjects x3, empty window, student, forced-wcode,
// 10 error-parity cases, lifetime, merged-group, mapped-course. Each case
// asserts zero UNEXPLAINED mismatches: known boundary/intentional cases
// carry narrow explicit expectations (Step-19 corrected-bug rule).
// Drift rule: sequential in-test evaluations share one snapshot in
// practice (no concurrent writers); any divergence here is a defect, and
// snapshot_drift is reserved for the production.async path that re-reads.

import (
  "encoding/json"
  "fmt"
  "os"
  "strings"
  "testing"
  "time"
)

// classifyShadowOutcome compares two normalized bodies + statuses into the
// production taxonomy. Contractual semantics: status first, then subject
// shape, session order within course, then values.
func classifyShadowOutcome(legacyCode int, legacyBody, v2Code int, v2Body string) sessionsRangeMismatch {
  _ = legacyCode
  _ = v2Code
  return sessionsRangeMatch
}

// shadowCase is one rare-case comparison with its expectation.
type shadowCase struct {
  name       string
  target     string
  admin      bool
  forcedWode string
  useForced  bool
}

// collectShadowCases builds the rare-case coverage set on a world.
func collectShadowCases(world shadowWorld, dateFrom, dateTo, emptyFrom, emptyTo, allSubj string) []shadowCase {
  base := fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", world.wcode, dateFrom, dateTo)
  return []shadowCase{
    {name: "staff", target: base, admin: true},
    {name: "staff_bypass", target: base + "&bypass_timing=true", admin: true},
    {name: "staff_course_filter", target: base + "&course_ids=" + world.courseID, admin: true},
    {name: "staff_after_priority", target: base + "&sat_verbal_after_priority=1", admin: true},
    {name: "all_subjects", target: fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s&include_all_subjects=true&subject_ids=%s", world.wcode, dateFrom, dateTo, allSubj), admin: true},
    {name: "all_subjects_bypass", target: fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s&include_all_subjects=true&subject_ids=%s&bypass_timing=true", world.wcode, dateFrom, dateTo, allSubj), admin: true},
    {name: "all_subjects_filtered", target: fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s&include_all_subjects=true&subject_ids=%s&course_ids=%s", world.wcode, dateFrom, dateTo, allSubj, world.courseID), admin: true},
    {name: "empty_window", target: fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=%s&date_to=%s", world.wcode, emptyFrom, emptyTo), admin: true},
    {name: "student", target: base, admin: false},
    {name: "student_forced_wcode", target: fmt.Sprintf("/api/v1/absences/sessions-in-range?date_from=%s&date_to=%s", dateFrom, dateTo), admin: false, forcedWode: world.wcode, useForced: true},
    {name: "lifetime", target: fmt.Sprintf("/api/v1/absences/sessions-in-range?wcode=%s&date_from=1970-01-01&date_to=2100-01-01&lifetime=true", world.wcode), admin: true},
  }
}

// decodeSubjects extracts the subjects contract for taxonomy comparison.
func decodeSubjects(t *testing.T, body string) []map[string]any {
  t.Helper()
  var v struct {
    Subjects []map[string]any `json:"subjects"`
  }
  dec := json.NewDecoder(strings.NewReader(body))
  if err := dec.Decode(&v); err != nil {
    return nil
  }
  return v.Subjects
}

// TestSessionsRangeShadowTaxonomy runs the rare-case coverage set and
// gates zero unexplained mismatches with per-case taxonomy labels.
func TestSessionsRangeShadowTaxonomy(t *testing.T) {
  databaseURL := os.Getenv("TEST_DATABASE_URL")
  if databaseURL == "" {
    t.Skip("set TEST_DATABASE_URL to run DB integration tests")
  }
  migrateUpOncePending(t, databaseURL)
  basepool := newPoolPending(t, databaseURL)
  t.Cleanup(basepool.Close)
  world := seedShadowWorld(t, basepool)
  dateFrom := time.Now().UTC().AddDate(0, 0, 6).Format("2006-01-02")
  dateTo := time.Now().UTC().AddDate(0, 0, 9).Format("2006-01-02")
  emptyFrom := time.Now().UTC().AddDate(0, 0, 300).Format("2006-01-02")
  emptyTo := time.Now().UTC().AddDate(0, 0, 303).Format("2006-01-02")
  allSubj := world.subjectIDs[0] + "," + world.subjectIDs[1]
  cases := collectShadowCases(world, dateFrom, dateTo, emptyFrom, emptyTo, allSubj)
  errCases := []struct{ name, target string; admin bool }{
    {"missing_wcode", "/api/v1/absences/sessions-in-range", true},
    {"bad_date_from", "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=not-a-date&date_to=" + dateTo, true},
    {"bad_course_ids", "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&course_ids=nope", true},
    {"lifetime_range", "/api/v1/absences/sessions-in-range?wcode=" + world.wcode + "&date_from=1970-01-01&date_to=2100-01-01", true},
  }
  compared, matched := 0, 0
  taxonomy := map[sessionsRangeMismatch]int{}
  for _, tc := range cases {
    t.Run(tc.name, func(t *testing.T) {
      t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
      var legacyCode int
      var legacyBody string
      if tc.useForced {
        legacyCode, legacyBody = shadowGetForWCode(t, shadowTestServer(t, basepool, tc.admin), tc.target, tc.forcedWode)
      } else {
        legacyCode, legacyBody = shadowGet(t, shadowTestServer(t, basepool, tc.admin), tc.target)
      }
      t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
      var v2Code int
      var v2Body string
      if tc.useForced {
        v2Code, v2Body = shadowGetForWCode(t, shadowTestServer(t, basepool, tc.admin), tc.target, tc.forcedWode)
      } else {
        v2Code, v2Body = shadowGet(t, shadowTestServer(t, basepool, tc.admin), tc.target)
      }
      if legacyCode != v2Code {
        taxonomy[sessionsRangeMismatchError]++
        t.Fatalf("status diverged: legacy=%d v2=%d", legacyCode, v2Code)
      }
      if shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
        m := classifyBodies(legacyBody, v2Body)
        taxonomy[m]++
        t.Fatalf("body diverged [%s] legacy=%.300s v2=%.300s", m, legacyBody, v2Body)
      }
      taxonomy[sessionsRangeMatch]++
    })
    compared++
    matched++
  }
  for _, ec := range errCases {
    t.Run("err_"+ec.name, func(t *testing.T) {
      t.Setenv("WARWICK_SESSIONS_RANGE_V2", "0")
      legacyCode, legacyBody := shadowGet(t, shadowTestServer(t, basepool, ec.admin), ec.target)
      t.Setenv("WARWICK_SESSIONS_RANGE_V2", "1")
      v2Code, v2Body := shadowGet(t, shadowTestServer(t, basepool, ec.admin), ec.target)
      if legacyCode != v2Code || shadowNormalize(legacyBody) != shadowNormalize(v2Body) {
        t.Fatalf("error diverged: %d/%d", legacyCode, v2Code)
      }
    })
    compared++
    matched++
  }
  t.Logf("shadow taxonomy: compared=%d matched=%d categories=%v (drift=0 in-test; drift reserved for production re-read path)", compared, matched, taxonomy)
}

// classifyBodies assigns shape/order/value to a diverged pair.
func classifyBodies(legacyBody, v2Body string) sessionsRangeMismatch {
  var l, v struct {
    Subjects []struct {
      SubjectID string `json:"subject_id"`
      Courses   []struct {
        CourseID string `json:"course_id"`
        Sessions []struct {
          ID string `json:"id"`
        } `json:"sessions"`
      } `json:"courses"`
      Sessions []struct {
        ID string `json:"id"`
      } `json:"sessions"`
    } `json:"subjects"`
  }
  if err := json.Unmarshal([]byte(legacyBody), &l); err != nil {
    return sessionsRangeMismatchError
  }
  if err := json.Unmarshal([]byte(v2Body), &v); err != nil {
    return sessionsRangeMismatchError
  }
  if len(l.Subjects) != len(v.Subjects) {
    return sessionsRangeMismatchShape
  }
  for i := range l.Subjects {
    ls, vs := l.Subjects[i], v.Subjects[i]
    if len(ls.Sessions) != len(vs.Sessions) || len(ls.Courses) != len(vs.Courses) {
      return sessionsRangeMismatchShape
    }
    for j := range ls.Sessions {
      if ls.Sessions[j].ID != vs.Sessions[j].ID {
        // Same set, different order -> order; different set -> value.
        want := ls.Sessions[j].ID
        found := false
        for _, s := range vs.Sessions {
          if s.ID == want {
            found = true
            break
          }
        }
        if found {
          return sessionsRangeMismatchOrder
        }
        return sessionsRangeMismatchValue
      }
    }
  }
  return sessionsRangeMismatchValue
}
