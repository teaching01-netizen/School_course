package absenceshttp

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

type calendarSitInStudentDTO struct {
	Wcode         string  `json:"wcode"`
	StudentName   *string `json:"student_name"`
	AbsenceID     string  `json:"absence_id"`
	FromCourseCode string `json:"from_course_code"`
	FromCourseName *string `json:"from_course_name"`
}

type calendarSessionBriefDTO struct {
	ID           string                   `json:"id"`
	CourseID     string                   `json:"course_id"`
	CourseCode   string                   `json:"course_code"`
	CourseName   string                   `json:"course_name"`
	SubjectName  *string                  `json:"subject_name"`
	StartAt      string                   `json:"start_at"`
	EndAt        string                   `json:"end_at"`
	RoomName     *string                  `json:"room_name"`
	TeacherName  *string                  `json:"teacher_name"`
	SitInStudents []calendarSitInStudentDTO `json:"sit_in_students,omitempty"`
}

type calendarAbsenceDTO struct {
	ID               string  `json:"id"`
	Wcode            string  `json:"wcode"`
	StudentName      *string `json:"student_name"`
	Status           string  `json:"status"`
	SubjectCode      *string `json:"subject_code"`
	SubjectName      *string `json:"subject_name"`
	DateFrom         string  `json:"date_from"`
	DateTo           string  `json:"date_to"`
	SitInMethod      *string `json:"sit_in_method"`
	SitInCourseCode  *string `json:"sit_in_course_code"`
	SitInCourseName  *string `json:"sit_in_course_name"`
	SitInSubjectName *string `json:"sit_in_subject_name"`
}

type calendarAbsenceDayDTO struct {
	Date     string               `json:"date"`
	Absences []calendarAbsenceDTO `json:"absences"`
}

type calendarResponseDTO struct {
	Sessions    []calendarSessionBriefDTO `json:"sessions"`
	AbsenceDays []calendarAbsenceDayDTO   `json:"absence_days"`
}

type calendarAbsenceEntry struct {
	ID  string
	DTO calendarAbsenceDTO
}

type absenceSessionDTO struct {
	ID         string  `json:"id"`
	SessionID  string  `json:"session_id"`
	CourseID   string  `json:"course_id"`
	CourseCode string  `json:"course_code"`
	CourseName string  `json:"course_name"`
	RoomName   *string `json:"room_name"`
	StartAt    string  `json:"start_at"`
	EndAt      string  `json:"end_at"`
}

type absenceTimelineDTO struct {
	ID        string          `json:"id"`
	Action    string          `json:"action"`
	ActorID   *string         `json:"actor_id"`
	ActorName *string         `json:"actor_name"`
	ActorRole string          `json:"actor_role"`
	Details   json.RawMessage `json:"details"`
	CreatedAt string          `json:"created_at"`
}

type managedAbsenceDTO struct {
	ID                  string               `json:"id"`
	Wcode               string               `json:"wcode"`
	StudentName         *string              `json:"student_name"`
	StudentEmail        *string              `json:"student_email"`
	StudentNickname     *string              `json:"student_nickname"`
	StudentPhone        *string              `json:"student_phone"`
	CourseID            string               `json:"course_id"`
	CourseCode          string               `json:"course_code"`
	CourseName          string               `json:"course_name"`
	SubjectID           *string              `json:"subject_id"`
	SubjectCode         *string              `json:"subject_code"`
	SubjectName         *string              `json:"subject_name"`
	DateFrom            string               `json:"date_from"`
	DateTo              string               `json:"date_to"`
	ReasonCategory      *string              `json:"reason_category"`
	Reason              *string              `json:"reason"`
	SitInMethod         *string              `json:"sit_in_method"`
	SitInCourseID       *string              `json:"sit_in_course_id"`
	SitInCourseCode     *string              `json:"sit_in_course_code"`
	SitInCourseName     *string              `json:"sit_in_course_name"`
	SitInSubjectName    *string              `json:"sit_in_subject_name"`
	Status              string               `json:"status"`
	AdminNotes          *string              `json:"admin_notes"`
	ReviewedBy          *string              `json:"reviewed_by"`
	ReviewedAt          *string              `json:"reviewed_at"`
	SitInOverridden     bool                 `json:"sit_in_overridden"`
	SitInOverriddenBy   *string              `json:"sit_in_overridden_by"`
	SitInOverrideReason *string              `json:"sit_in_override_reason"`
	Version             int32                `json:"version"`
	CreatedAt           string               `json:"created_at"`
	UpdatedAt           string               `json:"updated_at"`
	MissedSessions      []absenceSessionDTO  `json:"missed_sessions,omitempty"`
	SitIns              []absenceSessionDTO  `json:"sit_ins,omitempty"`
	Timeline            []absenceTimelineDTO `json:"timeline,omitempty"`
}

func stringPtrIfValid(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	value := v.String
	return &value
}

func (s *server) managedAbsenceDTO(row sqldb.ManagedAbsenceRow) managedAbsenceDTO {
	id, _ := s.a.UUIDString(row.ID)
	courseID, _ := s.a.UUIDString(row.CourseID)
	out := managedAbsenceDTO{
		ID:                  id,
		Wcode:               row.Wcode,
		StudentName:         stringPtrIfValid(row.StudentName),
		StudentEmail:        stringPtrIfValid(row.StudentEmail),
		StudentNickname:     stringPtrIfValid(row.StudentNickname),
		StudentPhone:        stringPtrIfValid(row.StudentPhone),
		CourseID:            courseID,
		CourseCode:          row.CourseCode,
		CourseName:          row.CourseName,
		DateFrom:            row.DateFrom.Time.Format("2006-01-02"),
		DateTo:              row.DateTo.Time.Format("2006-01-02"),
		ReasonCategory:      stringPtrIfValid(row.ReasonCategory),
		Reason:              stringPtrIfValid(row.Reason),
		SitInMethod:         stringPtrIfValid(row.SitInMethod),
		SitInCourseCode:     stringPtrIfValid(row.SitInCourseCode),
		SitInCourseName:     stringPtrIfValid(row.SitInCourseName),
		SitInSubjectName:    stringPtrIfValid(row.SitInSubjectName),
		Status:              row.Status,
		AdminNotes:          stringPtrIfValid(row.AdminNotes),
		SitInOverridden:     row.SitInOverridden,
		SitInOverrideReason: stringPtrIfValid(row.SitInOverrideReason),
		Version:             row.Version,
		CreatedAt:           row.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
		UpdatedAt:           row.UpdatedAt.Time.UTC().Format(time.RFC3339Nano),
	}
	if row.SubjectID.Valid {
		value, _ := s.a.UUIDString(row.SubjectID)
		out.SubjectID = &value
	}
	out.SubjectCode = stringPtrIfValid(row.SubjectCode)
	out.SubjectName = stringPtrIfValid(row.SubjectName)
	if row.SitInCourseID.Valid {
		value, _ := s.a.UUIDString(row.SitInCourseID)
		out.SitInCourseID = &value
	}
	if row.ReviewedBy.Valid {
		value, _ := s.a.UUIDString(row.ReviewedBy)
		out.ReviewedBy = &value
	}
	if row.ReviewedAt.Valid {
		value := row.ReviewedAt.Time.UTC().Format(time.RFC3339Nano)
		out.ReviewedAt = &value
	}
	if row.SitInOverriddenBy.Valid {
		value, _ := s.a.UUIDString(row.SitInOverriddenBy)
		out.SitInOverriddenBy = &value
	}
	return out
}

func (s *server) parseFilter(w http.ResponseWriter, r *http.Request, defaultLimit int32) (sqldb.AbsenceFilter, bool) {
	query := r.URL.Query()
	filter := sqldb.AbsenceFilter{Query: strings.TrimSpace(query.Get("query")), Status: strings.TrimSpace(query.Get("status")), Limit: defaultLimit, IDs: []pgtype.UUID{}}
	if filter.Status != "" && !validAbsenceStatus(filter.Status) {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_status", "Unsupported status")
		return filter, false
	}
	if value := strings.TrimSpace(query.Get("subject_id")); value != "" {
		id, err := s.a.ParseUUID(value)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_subject_id", "Invalid subject_id")
			return filter, false
		}
		filter.SubjectID = id
	}
	if value := strings.TrimSpace(query.Get("ids")); value != "" {
		values := strings.Split(value, ",")
		if len(values) > 1000 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_ids", "No more than 1000 selected absence IDs can be exported")
			return filter, false
		}
		for _, raw := range values {
			id, err := s.a.ParseUUID(strings.TrimSpace(raw))
			if err != nil {
				s.a.WriteErr(w, http.StatusBadRequest, "bad_ids", "Invalid selected absence ID")
				return filter, false
			}
			filter.IDs = append(filter.IDs, id)
		}
	}
	for raw, target := range map[string]*pgtype.Date{"date_from": &filter.DateFrom, "date_to": &filter.DateTo} {
		value := strings.TrimSpace(query.Get(raw))
		if value == "" {
			continue
		}
		parsed := parseDate(value)
		if !parsed.Valid {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_date", "Invalid date filter")
			return filter, false
		}
		*target = parsed
	}
	if value := strings.TrimSpace(query.Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_limit", "limit must be between 1 and 100")
			return filter, false
		}
		filter.Limit = int32(parsed)
	}
	if value := strings.TrimSpace(query.Get("offset")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_offset", "offset must be non-negative")
			return filter, false
		}
		filter.Offset = int32(parsed)
	}
	return filter, true
}

func validAbsenceStatus(status string) bool {
	switch status {
	case "pending", "reviewed", "actioned", "cancelled":
		return true
	default:
		return false
	}
}

func validTransition(from, to string) bool {
	switch from {
	case "pending":
		return to == "reviewed" || to == "cancelled"
	case "reviewed":
		return to == "actioned" || to == "pending" || to == "cancelled"
	case "actioned":
		return to == "reviewed" || to == "cancelled"
	default:
		return false
	}
}

func (s *server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	startRaw := strings.TrimSpace(r.URL.Query().Get("start"))
	endRaw := strings.TrimSpace(r.URL.Query().Get("end"))
	if startRaw == "" || endRaw == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_params", "start and end query params required (YYYY-MM-DD)")
		return
	}
	startDate, err := time.Parse("2006-01-02", startRaw)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_start", "start must be YYYY-MM-DD")
		return
	}
	endDate, err := time.Parse("2006-01-02", endRaw)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_end", "end must be YYYY-MM-DD")
		return
	}
	rangeStart := startDate.UTC()
	rangeEnd := endDate.Add(24*time.Hour - time.Nanosecond).UTC()

	sessionRows, err := s.deps.Q.CalendarSessionsInRange(r.Context(), rangeStart, rangeEnd)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}

	absRows, err := s.deps.Q.AbsenceDaysInRange(r.Context(), startDate, endDate)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}

	sessions := make([]calendarSessionBriefDTO, 0, len(sessionRows))
	for _, row := range sessionRows {
		id, _ := s.a.UUIDString(row.ID)
		courseID, _ := s.a.UUIDString(row.CourseID)
		sessions = append(sessions, calendarSessionBriefDTO{
			ID:          id,
			CourseID:    courseID,
			CourseCode:  row.CourseCode,
			CourseName:  row.CourseName,
			SubjectName: stringPtrIfValid(row.SubjectName),
			StartAt:     row.StartAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:       row.EndAt.Time.UTC().Format(time.RFC3339Nano),
			RoomName:    stringPtrIfValid(row.RoomName),
			TeacherName: stringPtrIfValid(row.TeacherName),
		})
	}

	entries := make([]calendarAbsenceEntry, 0, len(absRows))
	absenceIDs := make([]pgtype.UUID, 0, len(absRows))
	for _, row := range absRows {
		id, _ := s.a.UUIDString(row.ID)
		absenceIDs = append(absenceIDs, row.ID)
		entries = append(entries, calendarAbsenceEntry{
			ID: id,
			DTO: calendarAbsenceDTO{
				ID:               id,
				Wcode:            row.Wcode,
				StudentName:      stringPtrIfValid(row.StudentName),
				Status:           row.Status,
				SubjectCode:      stringPtrIfValid(row.SubjectCode),
				SubjectName:      stringPtrIfValid(row.SubjectName),
				DateFrom:         row.DateFrom.Time.Format("2006-01-02"),
				DateTo:           row.DateTo.Time.Format("2006-01-02"),
				SitInMethod:      stringPtrIfValid(row.SitInMethod),
				SitInCourseCode:  stringPtrIfValid(row.SitInCourseCode),
				SitInCourseName:  stringPtrIfValid(row.SitInCourseName),
				SitInSubjectName: stringPtrIfValid(row.SitInSubjectName),
			},
		})
	}

	missedDatesByAbsence := make(map[string]map[string]struct{})
	if len(absenceIDs) > 0 {
		missedRows, err := s.deps.Q.ManagedAbsenceMissedSessionsByAbsenceIDs(r.Context(), absenceIDs)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return
		}
		for _, missed := range missedRows {
			if !missed.AbsenceID.Valid || !missed.StartAt.Valid {
				continue
			}
			absID, _ := s.a.UUIDString(missed.AbsenceID)
			dateKey := missed.StartAt.Time.UTC().Format("2006-01-02")
			if missedDatesByAbsence[absID] == nil {
				missedDatesByAbsence[absID] = make(map[string]struct{})
			}
			missedDatesByAbsence[absID][dateKey] = struct{}{}
		}
	}

	absenceDays := buildCalendarAbsenceDays(entries, missedDatesByAbsence, rangeStart, rangeEnd)

	sessionIDs := make([]pgtype.UUID, 0, len(sessionRows))
	for _, row := range sessionRows {
		sessionIDs = append(sessionIDs, row.ID)
	}
	sitInBySession := make(map[string][]calendarSitInStudentDTO)
	if len(sessionIDs) > 0 {
		sitInRows, err := s.deps.Q.SitInsBySessionIDs(r.Context(), sessionIDs)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return
		}
		for _, si := range sitInRows {
			sessID, _ := s.a.UUIDString(si.SessionID)
			absID, _ := s.a.UUIDString(si.AbsenceID)
			sitInBySession[sessID] = append(sitInBySession[sessID], calendarSitInStudentDTO{
				Wcode:          si.Wcode,
				StudentName:    stringPtrIfValid(si.StudentName),
				AbsenceID:      absID,
				FromCourseCode: si.FromCourseCode,
				FromCourseName: stringPtrIfValid(si.FromCourseName),
			})
		}
	}
	for i := range sessions {
		sessions[i].SitInStudents = sitInBySession[sessions[i].ID]
	}

	s.a.WriteJSON(w, http.StatusOK, calendarResponseDTO{
		Sessions:    sessions,
		AbsenceDays: absenceDays,
	})
}

func buildCalendarAbsenceDays(entries []calendarAbsenceEntry, missedDatesByAbsence map[string]map[string]struct{}, rangeStart, rangeEnd time.Time) []calendarAbsenceDayDTO {
	startKey := rangeStart.UTC().Format("2006-01-02")
	endKey := rangeEnd.UTC().Format("2006-01-02")

	absByDate := make(map[string][]calendarAbsenceDTO)
	for _, entry := range entries {
		dates := make([]string, 0, len(missedDatesByAbsence[entry.ID]))
		for dateKey := range missedDatesByAbsence[entry.ID] {
			if dateKey < startKey || dateKey > endKey {
				continue
			}
			dates = append(dates, dateKey)
		}
		if len(dates) == 0 {
			dates = []string{entry.DTO.DateFrom}
		} else {
			sort.Strings(dates)
		}
		for _, dateKey := range dates {
			absByDate[dateKey] = append(absByDate[dateKey], entry.DTO)
		}
	}

	dates := make([]string, 0, len(absByDate))
	for d := range absByDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	absenceDays := make([]calendarAbsenceDayDTO, 0, len(dates))
	for _, d := range dates {
		absenceDays = append(absenceDays, calendarAbsenceDayDTO{Date: d, Absences: absByDate[d]})
	}
	return absenceDays
}

func (s *server) handleAbsenceInbox(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	filter, ok := s.parseFilter(w, r, 25)
	if !ok {
		return
	}
	rows, total, err := s.deps.Q.ManagedAbsenceList(r.Context(), filter)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	absenceIDs := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		absenceIDs = append(absenceIDs, row.ID)
	}
	missedByAbsence := make(map[pgtype.UUID][]sqldb.ManagedAbsenceSession, len(rows))
	if len(absenceIDs) > 0 {
		missedRows, err := s.deps.Q.ManagedAbsenceMissedSessionsByAbsenceIDs(r.Context(), absenceIDs)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return
		}
		for _, session := range missedRows {
			missedByAbsence[session.AbsenceID] = append(missedByAbsence[session.AbsenceID], session)
		}
	}
	items := make([]managedAbsenceDTO, 0, len(rows))
	for _, row := range rows {
		dto := s.managedAbsenceDTO(row)
		if missed := missedByAbsence[row.ID]; len(missed) > 0 {
			dto.MissedSessions = s.sessionDTO(missed)
		}
		items = append(items, dto)
	}
	subjectRows, err := s.deps.Q.SubjectListActive(r.Context())
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	subjects := make([]map[string]string, 0, len(subjectRows))
	for _, subject := range subjectRows {
		id, _ := s.a.UUIDString(subject.ID)
		subjects = append(subjects, map[string]string{"id": id, "code": subject.Code, "name": subject.Name})
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"subjects":    subjects,
		"total_count": total,
		"offset":      filter.Offset,
		"limit":       filter.Limit,
	})
}

func (s *server) timelineDTO(rows []sqldb.AbsenceAuditEntry) []absenceTimelineDTO {
	out := make([]absenceTimelineDTO, 0, len(rows))
	for _, row := range rows {
		id, _ := s.a.UUIDString(row.ID)
		dto := absenceTimelineDTO{
			ID:        id,
			Action:    row.Action,
			ActorName: stringPtrIfValid(row.ActorName),
			ActorRole: row.ActorRole,
			Details:   json.RawMessage(row.Details),
			CreatedAt: row.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
		}
		if row.ActorID.Valid {
			value, _ := s.a.UUIDString(row.ActorID)
			dto.ActorID = &value
		}
		out = append(out, dto)
	}
	return out
}

func (s *server) sessionDTO(rows []sqldb.ManagedAbsenceSession) []absenceSessionDTO {
	out := make([]absenceSessionDTO, 0, len(rows))
	for _, row := range rows {
		id, _ := s.a.UUIDString(row.ID)
		sessionID, _ := s.a.UUIDString(row.SessionID)
		courseID, _ := s.a.UUIDString(row.CourseID)
		out = append(out, absenceSessionDTO{
			ID: id, SessionID: sessionID, CourseID: courseID, CourseCode: row.CourseCode, CourseName: row.CourseName,
			RoomName: stringPtrIfValid(row.RoomName),
			StartAt:  row.StartAt.Time.UTC().Format(time.RFC3339Nano),
			EndAt:    row.EndAt.Time.UTC().Format(time.RFC3339Nano),
		})
	}
	return out
}

func (s *server) handleAbsenceGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}
	row, err := s.deps.Q.ManagedAbsenceGet(r.Context(), id)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	sessions, err := s.deps.Q.ManagedAbsenceSessions(r.Context(), id)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	missedSessions, err := s.deps.Q.ManagedAbsenceMissedSessions(r.Context(), id)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	timeline, err := s.deps.Q.AbsenceAuditList(r.Context(), id)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	out := s.managedAbsenceDTO(row)
	out.MissedSessions = s.sessionDTO(missedSessions)
	out.SitIns = s.sessionDTO(sessions)
	out.Timeline = s.timelineDTO(timeline)
	s.a.WriteJSON(w, http.StatusOK, out)
}

func (s *server) handleAbsenceTimeline(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}
	rows, err := s.deps.Q.AbsenceAuditList(r.Context(), id)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, s.timelineDTO(rows))
}

func actorID(userID [16]byte) pgtype.UUID {
	return pgtype.UUID{Bytes: userID, Valid: true}
}

func (s *server) writeStaleAbsence(w http.ResponseWriter) {
	s.a.WriteErr(w, http.StatusConflict, "stale_edit", "Absence was changed by another administrator; reload and try again")
}

func statusAuditAction(current, next string) string {
	if current == "reviewed" && next == "pending" {
		return "reopened"
	}
	return next
}

func (s *server) handleAbsenceStatusUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}
	var body struct {
		Status          string `json:"status"`
		Reason          string `json:"reason"`
		ExpectedVersion *int32 `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if !validAbsenceStatus(body.Status) {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_status", "Unsupported status")
		return
	}
	if body.ExpectedVersion == nil || *body.ExpectedVersion < 1 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version is required")
		return
	}
	if body.Status == "cancelled" && strings.TrimSpace(body.Reason) == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_reason", "Cancellation reason is required")
		return
	}
	settings, settingsErr := s.readAbsenceSettings(r)
	if settingsErr != nil {
		status, code, msg := s.a.ClassifyDBErr(settingsErr)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	adminID := actorID(user.ID)
	s.a.WithIdempotentTx(w, r, user.ID, "absences", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		current, err := qtx.ManagedAbsenceGet(r.Context(), id)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if current.Version != *body.ExpectedVersion {
			s.writeStaleAbsence(w)
			return 0, nil, pgx.ErrNoRows
		}
		if !validTransition(current.Status, body.Status) {
			s.a.WriteErr(w, http.StatusConflict, "invalid_transition", "Status transition is not permitted")
			return 0, nil, fmt.Errorf("invalid transition")
		}
		version, err := qtx.AbsenceStatusUpdate(r.Context(), id, body.Status, adminID, *body.ExpectedVersion)
		if err != nil {
			if sqldb.IsNoRows(err) {
				s.writeStaleAbsence(w)
			} else {
				status, code, message := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, message)
			}
			return 0, nil, err
		}
		if body.Status == "cancelled" {
			if err := qtx.AbsenceSitInsReplace(r.Context(), id, nil); err != nil {
				status, code, message := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, message)
				return 0, nil, err
			}
		}
		details := map[string]any{"from": current.Status, "to": body.Status}
		if body.Status == "cancelled" {
			details["reason"] = strings.TrimSpace(body.Reason)
		}
		action := statusAuditAction(current.Status, body.Status)
		if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{AbsenceID: id, Action: action, ActorID: adminID, Details: details}); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
			return 0, nil, err
		}
		_, err = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{ActorUserID: adminID, Action: "absence." + action, Payload: map[string]any{"absence_id": r.PathValue("id"), "from": current.Status, "to": body.Status}})
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write audit log")
			return 0, nil, err
		}
		if body.Status == "actioned" {
			recipients := successSMSPhones(current.ParentPhone, current.StudentPhone)
			if len(recipients) > 0 {
				sessions, sessErr := qtx.ManagedAbsenceSessions(r.Context(), id)
				if sessErr == nil {
					missed, missedErr := qtx.ManagedAbsenceMissedSessions(r.Context(), id)
					if missedErr == nil {
						sendSuccessSMS(s.deps.SMS, s.deps.Log, settings.Notifications.SmsSuccessTemplate, current, sessions, missed, recipients, s.deps.InstituteTZ)
					} else {
						if s.deps.Log != nil {
							s.deps.Log.Error("failed to load missed sessions for sms", "absence_id", r.PathValue("id"), "error", missedErr)
						}
						sendSuccessSMS(s.deps.SMS, s.deps.Log, settings.Notifications.SmsSuccessTemplate, current, sessions, nil, recipients, s.deps.InstituteTZ)
					}
				} else if s.deps.Log != nil {
					s.deps.Log.Error("failed to load absence sessions for sms", "absence_id", r.PathValue("id"), "error", sessErr)
				}
			}
		}
		return http.StatusOK, map[string]any{"status": body.Status, "version": version}, nil
	})
}

func (s *server) handleAbsenceNotesUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}
	var body struct {
		Notes           string `json:"notes"`
		ExpectedVersion *int32 `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.ExpectedVersion == nil || *body.ExpectedVersion < 1 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version is required")
		return
	}
	if len([]rune(body.Notes)) > 4000 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_notes", "Notes must not exceed 4000 characters")
		return
	}
	adminID := actorID(user.ID)
	s.a.WithIdempotentTx(w, r, user.ID, "absences", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		version, err := qtx.AbsenceNotesUpdate(r.Context(), id, strings.TrimSpace(body.Notes), *body.ExpectedVersion)
		if err != nil {
			if sqldb.IsNoRows(err) {
				s.writeStaleAbsence(w)
			} else {
				status, code, message := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, message)
			}
			return 0, nil, err
		}
		details := map[string]any{"has_note": strings.TrimSpace(body.Notes) != ""}
		if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{AbsenceID: id, Action: "note_added", ActorID: adminID, Details: details}); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
			return 0, nil, err
		}
		_, err = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{ActorUserID: adminID, Action: "absence.note_added", Payload: map[string]any{"absence_id": r.PathValue("id")}})
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write audit log")
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"version": version, "admin_notes": strings.TrimSpace(body.Notes)}, nil
	})
}

func parseUUIDs(s *server, values []string) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, 0, len(values))
	for _, value := range values {
		id, err := s.a.ParseUUID(value)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *server) handleSitInOverride(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}
	var body struct {
		Method          string   `json:"method"`
		SitInCourseID   *string  `json:"sit_in_course_id"`
		SitInSessionIDs []string `json:"sit_in_session_ids"`
		Reason          string   `json:"reason"`
		ExpectedVersion *int32   `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.Method != "auto" && body.Method != "zoom" && body.Method != "physical" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_method", "Method must be auto, zoom, or physical")
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_reason", "Override reason is required")
		return
	}
	if body.ExpectedVersion == nil || *body.ExpectedVersion < 1 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version is required")
		return
	}
	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	var selectedCourse pgtype.UUID
	var sessionIDs []pgtype.UUID
	if body.Method == "physical" {
		if body.SitInCourseID == nil || len(body.SitInSessionIDs) == 0 {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in", "Physical override requires a course and at least one session")
			return
		}
		selectedCourse, err = s.a.ParseUUID(*body.SitInCourseID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_sit_in_course_id", "Invalid sit_in_course_id")
			return
		}
		sessionIDs, err = parseUUIDs(s, body.SitInSessionIDs)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_session_id", "Invalid sit-in session ID")
			return
		}
		if len(sessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
			s.a.WriteErr(w, http.StatusBadRequest, "too_many_sessions", "Selected sit-in sessions exceed the configured maximum")
			return
		}
	}
	adminID := actorID(user.ID)
	s.a.WithIdempotentTx(w, r, user.ID, "absences", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		current, err := qtx.ManagedAbsenceGet(r.Context(), id)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if current.Version != *body.ExpectedVersion {
			s.writeStaleAbsence(w)
			return 0, nil, pgx.ErrNoRows
		}
		method := body.Method
		if method == "auto" {
			if !current.SubjectID.Valid {
				s.a.WriteErr(w, http.StatusConflict, "no_resolution", "Absence does not have a subject for automatic resolution")
				return 0, nil, fmt.Errorf("missing subject")
			}
			resolved, err := resolveSitIn(r.Context(), qtx, current.Wcode, current.SubjectID, current.DateFrom.Time, current.DateTo.Time)
			if err != nil || resolved == nil {
				s.a.WriteErr(w, http.StatusConflict, "no_resolution", "Automatic sit-in resolution is unavailable")
				if err == nil {
					err = fmt.Errorf("nil resolution")
				}
				return 0, nil, err
			}
			method = resolved.SitInMethod
			if resolved.SitInCourse != nil {
				selectedCourse, err = s.a.ParseUUID(resolved.SitInCourse.ID)
				if err != nil {
					return 0, nil, err
				}
			}
			sessionIDs = make([]pgtype.UUID, 0, len(resolved.PreSelected))
			for _, candidate := range resolved.PreSelected {
				sessionID, err := s.a.ParseUUID(candidate.ID)
				if err != nil {
					return 0, nil, err
				}
				sessionIDs = append(sessionIDs, sessionID)
			}
			if len(sessionIDs) > settings.SitIn.MaxSessionsPerAbsence {
				sessionIDs = sessionIDs[:settings.SitIn.MaxSessionsPerAbsence]
			}
		}
		if method == "zoom" {
			selectedCourse = pgtype.UUID{}
			sessionIDs = nil
		}
		if method == "physical" {
			count, err := qtx.ValidSitInSessionCount(r.Context(), id, selectedCourse, sessionIDs)
			if err != nil {
				status, code, message := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, message)
				return 0, nil, err
			}
			if count != len(sessionIDs) {
				s.a.WriteErr(w, http.StatusBadRequest, "invalid_sessions", "Sit-in sessions must be in the selected course and must not overlap the missed class")
				return 0, nil, fmt.Errorf("invalid sessions")
			}
		}
		version, err := qtx.AbsenceSitInUpdate(r.Context(), id, method, selectedCourse, adminID, strings.TrimSpace(body.Reason), *body.ExpectedVersion)
		if err != nil {
			if sqldb.IsNoRows(err) {
				s.writeStaleAbsence(w)
			} else {
				status, code, message := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, message)
			}
			return 0, nil, err
		}
		if err := qtx.AbsenceSitInsReplace(r.Context(), id, sessionIDs); err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		details := map[string]any{"from_method": current.SitInMethod.String, "to_method": method, "reason": strings.TrimSpace(body.Reason)}
		if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{AbsenceID: id, Action: "sit_in_overridden", ActorID: adminID, Details: details}); err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write absence timeline")
			return 0, nil, err
		}
		_, err = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{ActorUserID: adminID, Action: "absence.sit_in_overridden", Payload: map[string]any{"absence_id": r.PathValue("id"), "method": method}})
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write audit log")
			return 0, nil, err
		}
		return http.StatusOK, map[string]any{"version": version, "sit_in_method": method}, nil
	})
}

func (s *server) handleSitInCandidates(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	absenceID, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}
	courseID, err := s.a.ParseUUID(r.URL.Query().Get("course_id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_course_id", "Invalid course ID")
		return
	}
	rows, err := s.deps.Q.SitInCandidateSessions(r.Context(), absenceID, courseID)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id, _ := s.a.UUIDString(row.ID)
		capacityWarning := row.RoomCapacity.Valid && row.RoomCapacity.Int32 > 0 && float64(row.Occupancy)/float64(row.RoomCapacity.Int32) >= 0.9
		items = append(items, map[string]any{
			"id": id, "start_at": row.StartAt.Time.UTC().Format(time.RFC3339Nano), "end_at": row.EndAt.Time.UTC().Format(time.RFC3339Nano),
			"room_name": textValue(row.RoomName), "room_capacity": row.RoomCapacity.Int32, "occupancy": row.Occupancy, "capacity_warning": capacityWarning,
		})
	}
	s.a.WriteJSON(w, http.StatusOK, items)
}

func (s *server) handleAbsenceStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	stats, err := s.deps.Q.AbsenceStatsGet(r.Context())
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, stats)
}

func (s *server) handleBatchStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		IDs              []string         `json:"ids"`
		Status           string           `json:"status"`
		Reason           string           `json:"reason"`
		ExpectedVersions map[string]int32 `json:"expected_versions"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if len(body.IDs) == 0 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_ids", "ids array is required")
		return
	}
	if len(body.IDs) > 100 {
		s.a.WriteErr(w, http.StatusBadRequest, "too_many", "Maximum 100 absences per batch")
		return
	}
	if !validAbsenceStatus(body.Status) {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_status", "Unsupported status")
		return
	}

	ids := make([]pgtype.UUID, 0, len(body.IDs))
	expectedVersions := make(map[[16]byte]int32, len(body.IDs))
	for _, rawID := range body.IDs {
		id, err := s.a.ParseUUID(rawID)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
			return
		}
		ids = append(ids, id)
		if ver, ok := body.ExpectedVersions[rawID]; ok {
			expectedVersions[id.Bytes] = ver
		}
	}

	adminID := actorID(user.ID)

	tx, err := s.deps.DB.Begin(r.Context())
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	defer tx.Rollback(r.Context())

	qtx := s.deps.Q.WithTx(tx)
	results := qtx.AbsenceBatchStatusUpdate(r.Context(), ids, body.Status, adminID, expectedVersions, strings.TrimSpace(body.Reason))

	succeeded := make([]string, 0, len(results))
	failed := make([]map[string]any, 0)
	for _, res := range results {
		idStr, _ := s.a.UUIDString(res.ID)
		if res.Success {
			succeeded = append(succeeded, idStr)
		} else {
			failed = append(failed, map[string]any{"id": idStr, "error": res.Error})
		}
	}

	// Best-effort audit entries for succeeded items
	for _, idStr := range succeeded {
		pid, _ := s.a.ParseUUID(idStr)
		_ = qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{AbsenceID: pid, Action: body.Status, ActorID: adminID, Details: map[string]any{"batch": true}})
		_, _ = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{ActorUserID: adminID, Action: "absence.batch." + body.Status, Payload: map[string]any{"absence_id": idStr}})
	}

	if err := tx.Commit(r.Context()); err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"succeeded":       succeeded,
		"failed":          failed,
		"total_processed": len(results),
	})
}

func (s *server) handleAbsenceDashboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	from, err := time.Parse("2006-01", month)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_month", "month must use YYYY-MM format")
		return
	}
	to := from.AddDate(0, 1, 0)
	stats, err := s.deps.Q.AbsenceStatsForRange(r.Context(), from, to)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	subjects, reasons, err := s.deps.Q.AbsenceDashboardBreakdowns(r.Context(), from, to)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"month": month, "stats": stats, "subjects": subjects, "reasons": reasons})
}

func (s *server) handleAbsenceExport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	filter, ok := s.parseFilter(w, r, 100)
	if !ok {
		return
	}
	filter.Limit = 10000
	filter.Offset = 0
	rows, _, err := s.deps.Q.ManagedAbsenceList(r.Context(), filter)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="absences-%s.csv"`, time.Now().Format("2006-01-02")))
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"WCode", "StudentName", "Subject", "Course", "DateFrom", "DateTo", "Reason", "SitInMethod", "SitInCourse", "Status", "SubmittedAt", "ReviewedAt", "ReviewedBy"})
	for _, row := range rows {
		reviewedAt := ""
		if row.ReviewedAt.Valid {
			reviewedAt = row.ReviewedAt.Time.UTC().Format(time.RFC3339)
		}
		reviewedBy := ""
		if row.ReviewedBy.Valid {
			reviewedBy, _ = s.a.UUIDString(row.ReviewedBy)
		}
		_ = writer.Write([]string{
			row.Wcode, textValue(row.StudentName), textValue(row.SubjectCode), row.CourseCode,
			row.DateFrom.Time.Format("2006-01-02"), row.DateTo.Time.Format("2006-01-02"), textValue(row.Reason),
			textValue(row.SitInMethod), textValue(row.SitInCourseCode), row.Status,
			row.CreatedAt.Time.UTC().Format(time.RFC3339), reviewedAt, reviewedBy,
		})
	}
	writer.Flush()
}

func textValue(value pgtype.Text) string {
	if value.Valid {
		return value.String
	}
	return ""
}

type reasonCategory struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type absenceFormSettings struct {
	MaxDateRangeDays    int              `json:"max_date_range_days"`
	RequireReason       bool             `json:"require_reason"`
	ReasonCategories    []reasonCategory `json:"reason_categories"`
	AllowFreeTextReason bool             `json:"allow_free_text_reason"`
	IntroText           string           `json:"intro_text"`
	ConfirmationText    string           `json:"confirmation_text"`
}

type absenceSitInSettings struct {
	AutoResolveEnabled    bool   `json:"auto_resolve_enabled"`
	ZoomDescription       string `json:"zoom_description"`
	MaxSessionsPerAbsence int    `json:"max_sessions_per_absence"`
}

type absenceNotificationsSettings struct {
	SmsParentEnabled      bool   `json:"sms_parent_enabled"`
	SmsParentTemplate     string `json:"sms_parent_template"`
	SmsSuccessTemplate    string `json:"sms_success_template"`
	AllowSubmitWithoutOtp bool   `json:"allow_submit_without_otp"`
}

type adminContactSettings struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
	Hours string `json:"hours"`
}

type studentSelfServiceSettings struct {
	CanViewOwn   bool `json:"can_view_own"`
	CanCancelOwn bool `json:"can_cancel_own"`
}

type absenceSettings struct {
	Form               absenceFormSettings          `json:"form"`
	SitIn              absenceSitInSettings         `json:"sit_in"`
	Notifications      absenceNotificationsSettings `json:"notifications"`
	AdminContact       adminContactSettings         `json:"admin_contact"`
	StudentSelfService studentSelfServiceSettings   `json:"student_self_service"`
}

func defaultAbsenceSettings() absenceSettings {
	return absenceSettings{
		Form: absenceFormSettings{
			MaxDateRangeDays: 30, RequireReason: false, AllowFreeTextReason: true,
			ReasonCategories: []reasonCategory{{Value: "medical", Label: "Medical"}, {Value: "family", Label: "Family"}, {Value: "transport", Label: "Transport"}, {Value: "other", Label: "Other"}},
		},
		SitIn: absenceSitInSettings{AutoResolveEnabled: true, ZoomDescription: "Zoom session - no physical class attendance required.", MaxSessionsPerAbsence: 10},
		Notifications: absenceNotificationsSettings{
			SmsParentEnabled:      true,
			SmsParentTemplate:     "Your Warwick verification code is {{code}}.",
			SmsSuccessTemplate:    "Warwick Institute: {{nickname}} ได้แจ้งลาเรียน {{absence_summary}} และมีกำหนดเข้าเรียนชดเชย {{sit_in_summary}} ทางสถาบันจึงเรียนมาเพื่อโปรดทราบ",
			AllowSubmitWithoutOtp: false,
		},
	}
}

func parseAbsenceSettings(raw []byte) absenceSettings {
	settings := defaultAbsenceSettings()
	var policies struct {
		Form               *absenceFormSettings          `json:"form"`
		SitIn              *absenceSitInSettings         `json:"sit_in"`
		Notifications      *absenceNotificationsSettings `json:"notifications"`
		AdminContact       *adminContactSettings         `json:"admin_contact"`
		StudentSelfService *studentSelfServiceSettings   `json:"student_self_service"`
		Zoom               *sqldb.ZoomConfig             `json:"zoom"`
	}
	if json.Unmarshal(raw, &policies) != nil {
		return settings
	}
	if policies.Form != nil {
		settings.Form = *policies.Form
	}
	if policies.SitIn != nil {
		settings.SitIn = *policies.SitIn
	} else if policies.Zoom != nil && policies.Zoom.Description != "" {
		settings.SitIn.ZoomDescription = policies.Zoom.Description
	}
	if policies.Notifications != nil {
		settings.Notifications = *policies.Notifications
	}
	if policies.AdminContact != nil {
		settings.AdminContact = *policies.AdminContact
	}
	if policies.StudentSelfService != nil {
		settings.StudentSelfService = *policies.StudentSelfService
	}
	return settings
}

func validateAbsenceSettings(settings absenceSettings) error {
	if settings.Form.MaxDateRangeDays < 1 || settings.Form.MaxDateRangeDays > 365 {
		return fmt.Errorf("max_date_range_days must be between 1 and 365")
	}
	if len(settings.Form.ReasonCategories) == 0 {
		return fmt.Errorf("at least one reason category is required")
	}
	seen := make(map[string]bool)
	for _, category := range settings.Form.ReasonCategories {
		key := strings.TrimSpace(category.Value)
		if key == "" || strings.TrimSpace(category.Label) == "" || seen[key] {
			return fmt.Errorf("reason categories need unique values and labels")
		}
		seen[key] = true
	}
	if len([]rune(settings.Form.IntroText)) > 500 || len([]rune(settings.Form.ConfirmationText)) > 500 {
		return fmt.Errorf("student-facing text must not exceed 500 characters")
	}
	if settings.SitIn.MaxSessionsPerAbsence < 1 || settings.SitIn.MaxSessionsPerAbsence > 100 {
		return fmt.Errorf("max_sessions_per_absence must be between 1 and 100")
	}
	if len([]rune(settings.Notifications.SmsParentTemplate)) > 500 {
		return fmt.Errorf("sms_parent_template must not exceed 500 characters")
	}
	if len([]rune(settings.Notifications.SmsSuccessTemplate)) > 500 {
		return fmt.Errorf("sms_success_template must not exceed 500 characters")
	}
	if len([]rune(settings.AdminContact.Email)) > 200 || len([]rune(settings.AdminContact.Phone)) > 50 || len([]rune(settings.AdminContact.Hours)) > 120 {
		return fmt.Errorf("admin contact fields are too long")
	}
	return nil
}

func (s *server) readAbsenceSettings(r *http.Request) (absenceSettings, error) {
	row, err := s.deps.Q.AppSettingsGetWithPolicies(r.Context())
	if err != nil {
		return absenceSettings{}, err
	}
	return parseAbsenceSettings(row.AbsencePolicies), nil
}

func (s *server) handleFormConfigGet(w http.ResponseWriter, r *http.Request) {
	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"form":          settings.Form,
		"sit_in":        settings.SitIn,
		"notifications": settings.Notifications,
		"admin_contact": settings.AdminContact,
	})
}

func (s *server) handleAbsenceSettingsGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, message := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, message)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, settings)
}

func (s *server) handleAbsenceSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body absenceSettings
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if err := validateAbsenceSettings(body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_settings", err.Error())
		return
	}
	raw, _ := json.Marshal(body)
	adminID := actorID(user.ID)
	s.a.WithIdempotentTx(w, r, user.ID, "absence-settings", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		current, err := qtx.AppSettingsGetWithPolicies(r.Context())
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		merged := deepMergeAbsencePolicies(current.AbsencePolicies, raw)
		if err := qtx.AppSettingsUpdateAbsencePolicies(r.Context(), merged); err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		_, err = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{ActorUserID: adminID, Action: "absence.settings_updated", Payload: map[string]any{"max_date_range_days": body.Form.MaxDateRangeDays}})
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write audit log")
			return 0, nil, err
		}
		return http.StatusOK, body, nil
	})
}

func (s *server) handleAbsenceDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid absence ID")
		return
	}
	var body struct {
		ExpectedVersion *int32 `json:"expected_version"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if body.ExpectedVersion == nil || *body.ExpectedVersion < 1 {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_expected_version", "expected_version is required")
		return
	}
	adminID := actorID(user.ID)
	s.a.WithIdempotentTx(w, r, user.ID, "absences", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		current, err := qtx.ManagedAbsenceGet(r.Context(), id)
		if err != nil {
			status, code, message := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, message)
			return 0, nil, err
		}
		if current.Version != *body.ExpectedVersion {
			s.writeStaleAbsence(w)
			return 0, nil, pgx.ErrNoRows
		}

		// ON DELETE CASCADE on absence_sit_ins, absence_missed_sessions,
		// and absence_audit_log handles child row cleanup atomically.
		// Version check in WHERE clause provides DB-level optimistic locking.
		if _, err := qtx.AbsenceHardDelete(r.Context(), id, *body.ExpectedVersion); err != nil {
			if sqldb.IsNoRows(err) {
				s.writeStaleAbsence(w)
			} else {
				status, code, message := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, message)
			}
			return 0, nil, err
		}

		_, err = qtx.AuditInsert(r.Context(), sqldb.AuditInsertParams{
			ActorUserID: adminID,
			Action:      "absence.hard_deleted",
			Payload:     map[string]any{"absence_id": r.PathValue("id"), "wcode": current.Wcode},
		})
		if err != nil {
			s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Could not write audit log")
			return 0, nil, err
		}
		return http.StatusOK, map[string]string{"status": "deleted"}, nil
	})
}
