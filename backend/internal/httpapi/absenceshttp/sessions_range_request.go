package absenceshttp

// Impossible-state request types.
//
// Instead of threading (adminRequest, requireAdmin, includeAllSubjects,
// bypassTiming, subjectIDs, courseIDs, wcode) booleans through one handler,
// parse + authorize once into exactly one of:
//
//	StaffSessionLookup     (enrolled modes, may bypass timing)
//	StudentSessionLookup   (bookable modes, timing always enforced)
//	StaffAllSubjectsLookup (special sit-in, subject list, timing enforced)
//
// A StudentSessionLookup has NO bypassTiming capability: the type exposes
// bypassTiming() == false by construction, so no execution path can upgrade
// authority via query parameters.

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// sessionsRangeLookup is the sealed lookup union.
type sessionsRangeLookup interface {
	studentWCode() string
	courseAllowList() map[string]bool
	bypassTiming() bool
	satAfterPriority() int
	isStudent() bool
	isAllSubjects() bool
}

// StaffSessionLookup lists a student enrolled sessions with staff authority.
type StaffSessionLookup struct {
	WCode            string
	CourseIDs        map[string]bool
	BypassTiming     bool
	SatAfterPriority int
}

func (l StaffSessionLookup) studentWCode() string             { return l.WCode }
func (l StaffSessionLookup) courseAllowList() map[string]bool { return l.CourseIDs }
func (l StaffSessionLookup) bypassTiming() bool               { return l.BypassTiming }
func (l StaffSessionLookup) satAfterPriority() int            { return l.SatAfterPriority }
func (l StaffSessionLookup) isStudent() bool                  { return false }
func (l StaffSessionLookup) isAllSubjects() bool              { return false }

// StudentSessionLookup lists the caller own bookable sessions. Identity comes
// from authenticated session state (forcedWCode); the type cannot represent
// bypassTiming or all-subjects.
type StudentSessionLookup struct {
	WCode            string
	CourseIDs        map[string]bool
	SatAfterPriority int
}

func (l StudentSessionLookup) studentWCode() string             { return l.WCode }
func (l StudentSessionLookup) courseAllowList() map[string]bool { return l.CourseIDs }
func (l StudentSessionLookup) bypassTiming() bool               { return false }
func (l StudentSessionLookup) satAfterPriority() int            { return l.SatAfterPriority }
func (l StudentSessionLookup) isStudent() bool                  { return true }
func (l StudentSessionLookup) isAllSubjects() bool              { return false }

// StaffAllSubjectsLookup is the special sit-in lookup: sessions for explicit
// subjects with no enrollment predicate. Staff-only by construction.
type StaffAllSubjectsLookup struct {
	WCode            string
	SubjectIDs       []string
	CourseIDs        map[string]bool
	BypassTiming     bool
	SatAfterPriority int
}

func (l StaffAllSubjectsLookup) studentWCode() string             { return l.WCode }
func (l StaffAllSubjectsLookup) courseAllowList() map[string]bool { return l.CourseIDs }
func (l StaffAllSubjectsLookup) bypassTiming() bool               { return l.BypassTiming }
func (l StaffAllSubjectsLookup) satAfterPriority() int            { return l.SatAfterPriority }
func (l StaffAllSubjectsLookup) isStudent() bool                  { return false }
func (l StaffAllSubjectsLookup) isAllSubjects() bool              { return true }

// sessionsRangeWindow is the parsed half-open instant range.
type sessionsRangeWindow struct {
	from        time.Time
	toExclusive time.Time
}

// sessionsRangePrelim is the pre-settings parse output: identity, window,
// and authority. Settings load between prelim and finalize to preserve the
// legacy order (param validation -> admin gate -> settings -> range cap ->
// filters), so contract tests observing no-data-access-before-validation
// keep passing.
type sessionsRangePrelim struct {
	wcode         string
	dateFrom      time.Time
	dateTo        time.Time
	rangeProvided bool
	window        sessionsRangeWindow
	adminRequest  bool
	studentCall   bool
}

// parseSessionsRangePrelim validates identity/dates/authority with zero data
// access. It writes the error response and returns ok=false on rejection,
// preserving every legacy error code.
func parseSessionsRangePrelim(
	s *server,
	w http.ResponseWriter,
	r *http.Request,
	forcedWCode string,
	requireAdmin bool,
) (sessionsRangePrelim, bool) {
	var zero sessionsRangePrelim
	wcode := normalizeWCode(forcedWCode)
	studentCall := forcedWCode != "" || !requireAdmin
	if wcode == "" {
		wcode = normalizeWCode(r.URL.Query().Get("wcode"))
	}
	dateFromStr := r.URL.Query().Get("date_from")
	dateToStr := r.URL.Query().Get("date_to")
	if wcode == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_params", "wcode is required")
		return zero, false
	}
	dateFromProvided := dateFromStr != ""
	dateToProvided := dateToStr != ""
	if dateFromProvided != dateToProvided {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_date_range", "date_from and date_to must be provided together")
		return zero, false
	}
	dateRangeProvided := dateFromProvided && dateToProvided
	var dateFrom, dateTo time.Time
	var err error
	if dateRangeProvided {
		dateFrom, err = parseInstituteLocalDate(dateFromStr, s.deps.InstituteTZ)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date_from", "Invalid date_from, use YYYY-MM-DD")
			return zero, false
		}
		dateTo, err = parseInstituteLocalDate(dateToStr, s.deps.InstituteTZ)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date_to", "Invalid date_to, use YYYY-MM-DD")
			return zero, false
		}
		if dateTo.Before(dateFrom) {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date_range", "date_to must be on or after date_from")
			return zero, false
		}
	} else {
		now := time.Now()
		dateFrom = now.AddDate(0, 0, -30)
		dateTo = now.AddDate(0, 0, 90)
	}

	adminRequest := isAdminRequest(s.deps.Auth, r)
	if requireAdmin && !adminRequest {
		s.a.WriteErr(w, http.StatusUnauthorized, "unauthorized", "Staff authorization is required")
		return zero, false
	}
	window := sessionsRangeWindow{from: dateFrom, toExclusive: dateTo.AddDate(0, 0, 1)}
	return sessionsRangePrelim{wcode: wcode, dateFrom: dateFrom, dateTo: dateTo, rangeProvided: dateRangeProvided, window: window, adminRequest: adminRequest, studentCall: studentCall}, true
}

// finalizeSessionsRangeLookup applies the range cap (needs settings) and
// builds the lookup union. Preserves every legacy error code.
func finalizeSessionsRangeLookup(
	s *server,
	w http.ResponseWriter,
	r *http.Request,
	pre sessionsRangePrelim,
	settings absenceSettings,
) (sessionsRangeLookup, bool) {
	if pre.rangeProvided {
		days := int(pre.dateTo.Sub(pre.dateFrom).Hours() / 24)
		maxRangeDays := maxRangeDaysForLookup(settings, pre.adminRequest)
		if days > maxRangeDays {
			s.a.WriteErr(w, http.StatusBadRequest, "date_range_exceeded",
				"Date range must be "+strconv.Itoa(maxRangeDays)+" days or less")
			return nil, false
		}
	}

	allowedCourseIDs := map[string]bool{}
	if raw := strings.TrimSpace(r.URL.Query().Get("course_ids")); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			id, err := s.a.ParseUUID(value)
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_course_ids", "Invalid course_ids filter")
				return nil, false
			}
			courseID, _ := s.a.UUIDString(id)
			allowedCourseIDs[courseID] = true
		}
	}
	satVerbalAfterPriority := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("sat_verbal_after_priority")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sat_verbal_after_priority", "sat_verbal_after_priority must be a non-negative integer")
			return nil, false
		}
		satVerbalAfterPriority = value
	}
	bypassTiming := strings.TrimSpace(r.URL.Query().Get("bypass_timing")) == "true"
	includeAllSubjects := strings.TrimSpace(r.URL.Query().Get("include_all_subjects")) == "true"
	subjectIDFilter, err := parseSubjectIDFilter(s.a, strings.TrimSpace(r.URL.Query().Get("subject_ids")))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_ids", "Invalid subject_ids filter")
		return nil, false
	}
	if includeAllSubjects {
		if !pre.adminRequest {
			s.a.WriteErr(w, http.StatusForbidden, "admin_required", "Only staff can load all subject sessions")
			return nil, false
		}
		if len(subjectIDFilter) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_ids", "subject_ids are required when loading all subject sessions")
			return nil, false
		}
	}

	if includeAllSubjects {
		// Legacy honors bypass_timing in all-subjects mode (staff-only).
		return StaffAllSubjectsLookup{WCode: pre.wcode, SubjectIDs: subjectIDFilter, CourseIDs: allowedCourseIDs, BypassTiming: bypassTiming, SatAfterPriority: satVerbalAfterPriority}, true
	}
	if pre.studentCall && !pre.adminRequest {
		return StudentSessionLookup{WCode: pre.wcode, CourseIDs: allowedCourseIDs, SatAfterPriority: satVerbalAfterPriority}, true
	}
	return StaffSessionLookup{WCode: pre.wcode, CourseIDs: allowedCourseIDs, BypassTiming: bypassTiming, SatAfterPriority: satVerbalAfterPriority}, true
}

// parseSessionsRangeLookup validates the full parameter set with
// caller-provided settings. Kept for compatibility; the handler prefers the
// split prelim/finalize order (settings load between them).
func parseSessionsRangeLookup(
	s *server,
	w http.ResponseWriter,
	r *http.Request,
	forcedWCode string,
	requireAdmin bool,
	settings absenceSettings,
) (sessionsRangeLookup, sessionsRangeWindow, bool) {
	var zero sessionsRangeWindow
	pre, ok := parseSessionsRangePrelim(s, w, r, forcedWCode, requireAdmin)
	if !ok {
		return nil, zero, false
	}
	lookup, ok := finalizeSessionsRangeLookup(s, w, r, pre, settings)
	if !ok {
		return nil, zero, false
	}
	return lookup, pre.window, true
}
