package absenceshttp

// Step-23 production observability + safe shadow comparison.
//
// Telemetry: every sessions-in-range response (V1 or V2) records one
// sessionsRangeTelemetry value — endpoint mode, implementation version,
// phase durations, returned sessions/courses, candidates, payload bytes.
// Query/round-trip counts ride the existing batchCountingTracer in tests;
// in production the pool tracer (if configured) owns transport counts, so
// the record carries what the HANDLER knows (no invented transport data).
// No student identifiers, no wcodes, no payload bodies in telemetry.
//
// Shadow: compareSessionsRangeShadow runs the non-authoritative path
// in-process on the same facts + clock and classifies the outcome into a
// sessionsRangeMismatch taxonomy. Concurrent-state changes between the
// two evaluations classify as snapshot_drift (investigated separately),
// never as confirmed semantic defects. Bounded by construction: the
// shadow runs against ALREADY-FETCHED facts (no extra DB load), with an
// explicit opt-in env gate (WARWICK_SESSIONS_RANGE_SHADOW=1) and a payload
// cap (shadow skipped above shadowMaxPayloadSessions).

import (
  "net/http"
  "os"
  "strings"
  "time"
)

// sessionsRangeTelemetry is the per-request observability record. Field
// order is stable for log encoding; all counts are exact handler-known
// values, durations are wall-clock per phase.
// Privacy: no wcode, no student ID, no names, no payload content.
type sessionsRangeTelemetry struct {
  Mode             string `json:"mode"`
  ImplVersion      string `json:"impl"`
  StaffFacing      bool   `json:"staff_facing"`
  AllSubjects      bool   `json:"all_subjects"`
  Lifetime         bool   `json:"lifetime"`
  BypassTiming     bool   `json:"bypass_timing"`
  Subjects         int    `json:"subjects"`
  Courses          int    `json:"courses"`
  Sessions         int    `json:"sessions"`
  Candidates       int    `json:"candidates"`
  PayloadBytes     int    `json:"payload_bytes"`
  SettingsMS       int64  `json:"settings_ms"`
  FactsMS          int64  `json:"facts_ms"`
  DomainMS         int64  `json:"domain_ms"`
  SerializeMS      int64  `json:"serialize_ms"`
  TotalMS          int64  `json:"total_ms"`
  ShadowCompared   bool   `json:"shadow_compared"`
  ShadowMismatch   string `json:"shadow_mismatch,omitempty"`
}

// sessionsRangeMode classifies the request for telemetry without touching
// identity: staff-all-subjects, staff-enrolled, or student.
func sessionsRangeMode(allSubjects, staffFacing bool) string {
  if allSubjects {
    return "staff_all_subjects"
  }
  if staffFacing {
    return "staff_enrolled"
  }
  return "student"
}

// sessionsRangeShadowEnabled is the explicit opt-in for in-process shadow
// evaluation. Default OFF; set WARWICK_SESSIONS_RANGE_SHADOW=1 to enable.
// Shadow never changes the authoritative response.
func sessionsRangeShadowEnabled() bool {
  v, ok := os.LookupEnv("WARWICK_SESSIONS_RANGE_SHADOW")
  if !ok {
    return false
  }
  switch strings.ToLower(strings.TrimSpace(v)) {
  case "1", "true", "on":
    return true
  }
  return false
}

// shadowMaxPayloadSessions bounds shadow work: above this many display
// sessions the shadow comparison is skipped (logged as shadow_skipped) so
// a pathological payload cannot double domain CPU.
const shadowMaxPayloadSessions = 5000

// sessionsRangeMismatch classifies a shadow comparison outcome. The empty
// string means agreement. snapshot_drift covers concurrent-state change
// between evaluations (DB re-read inside legacy path); it is a signal to
// investigate, never a confirmed semantic defect.
type sessionsRangeMismatch string

const (
  sessionsRangeMatch            sessionsRangeMismatch = ""
  sessionsRangeMismatchShape    sessionsRangeMismatch = "shape"
  sessionsRangeMismatchOrder    sessionsRangeMismatch = "order"
  sessionsRangeMismatchValue    sessionsRangeMismatch = "value"
  sessionsRangeMismatchError    sessionsRangeMismatch = "error"
  sessionsRangeMismatchSkipped  sessionsRangeMismatch = "shadow_skipped"
  sessionsRangeMismatchDrift    sessionsRangeMismatch = "snapshot_drift"
)

// shadowClock skew budget: legacy reads one now per stage (Step-18 parity
// note), so a session exactly at a timing/cutoff boundary may
// legitimately differ when the two paths read the clock microseconds
// apart. The comparator receives the SAME now for both evaluations, so a
// residual difference after clock pinning is a real defect, not drift —
// drift applies only when the shadow path re-reads the DB.
const shadowSameClockNote = "both evaluations share one now; boundary flips are defects, not drift"

func (s *server) emitSessionsRangeTelemetry(r *http.Request, tele sessionsRangeTelemetry) {
  if s.deps.Log == nil {
    return
  }
  s.deps.Log.Info("sessions_range_response",
    "mode", tele.Mode,
    "impl", tele.ImplVersion,
    "staff_facing", tele.StaffFacing,
    "all_subjects", tele.AllSubjects,
    "lifetime", tele.Lifetime,
    "bypass_timing", tele.BypassTiming,
    "subjects", tele.Subjects,
    "courses", tele.Courses,
    "sessions", tele.Sessions,
    "candidates", tele.Candidates,
    "payload_bytes", tele.PayloadBytes,
    "settings_ms", tele.SettingsMS,
    "facts_ms", tele.FactsMS,
    "domain_ms", tele.DomainMS,
    "serialize_ms", tele.SerializeMS,
    "total_ms", tele.TotalMS,
    "shadow_compared", tele.ShadowCompared,
    "shadow_mismatch", tele.ShadowMismatch,
  )
  _ = r
}

func countSitInCandidates(courses []courseJSON) int {
  n := 0
  for _, c := range courses {
    if c.SitIn == nil {
      continue
    }
    n += len(c.SitIn.AvailableSessions) + len(c.SitIn.MissedSessions)
  }
  return n
}

var _ = shadowSameClockNote
var _ = time.Millisecond
