package teacherhttp

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type server struct {
	deps httpdeps.Deps
	a    httpadapter.Adapter
}

func Register(mux *http.ServeMux, deps httpdeps.Deps) {
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	mux.HandleFunc("GET /api/v1/teacher/dashboard", s.handleDashboard)
}

type teacherInfoDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type sitInVisitorDTO struct {
	Wcode               string  `json:"wcode"`
	Nickname            *string `json:"nickname"`
	StudentName         *string `json:"student_name"`
	FromCourseCode      string  `json:"from_course_code"`
	FromSubjectName     *string `json:"from_subject_name"`
	AbsenceID           string  `json:"absence_id"`
	SessionStartAt      string  `json:"session_start_at"`
	SessionEndAt        string  `json:"session_end_at"`
	AbsentSubjectName   *string `json:"absent_subject_name"`
}

type absentStudentDTO struct {
	Wcode       string  `json:"wcode"`
	Nickname    *string `json:"nickname"`
	StudentName *string `json:"student_name"`
	AbsenceID   string  `json:"absence_id"`
	CreatedAt   *string `json:"created_at"`
}

type sessionDTO struct {
	ID            string             `json:"id"`
	CourseID      string             `json:"course_id"`
	CourseCode    string             `json:"course_code"`
	CourseName    string             `json:"course_name"`
	SubjectName   *string            `json:"subject_name"`
	StartAt       string             `json:"start_at"`
	EndAt         string             `json:"end_at"`
	RoomName      *string            `json:"room_name"`
	AbsentCount   int                `json:"absent_count"`
	AbsentStudents []absentStudentDTO `json:"absent_students"`
	SitInVisitors []sitInVisitorDTO  `json:"sit_in_visitors"`
}

type weeklySummaryDTO struct {
	TotalSessions int `json:"total_sessions"`
	TotalAbsences int `json:"total_absences"`
	TotalSitIns   int `json:"total_sit_ins"`
}

type pendingAbsenceRequestDTO struct {
	ID             string  `json:"id"`
	Wcode          string  `json:"wcode"`
	StudentName    *string `json:"student_name"`
	Nickname       *string `json:"nickname"`
	CourseCode     string  `json:"course_code"`
	CourseName     string  `json:"course_name"`
	SubjectName    *string `json:"subject_name"`
	DateFrom       string  `json:"date_from"`
	DateTo         string  `json:"date_to"`
	Reason         *string `json:"reason"`
	ReasonCategory *string `json:"reason_category"`
	CreatedAt      string  `json:"created_at"`
}

type dashboardResponse struct {
	WeekStart              string                      `json:"week_start"`
	WeekEnd                string                      `json:"week_end"`
	Teacher                teacherInfoDTO              `json:"teacher"`
	Sessions               []sessionDTO                `json:"sessions"`
	Summary                weeklySummaryDTO            `json:"summary"`
	PendingAbsenceRequests []pendingAbsenceRequestDTO  `json:"pending_absence_requests"`
}

func getMonday(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		return t.AddDate(0, 0, -6)
	}
	diff := -int(weekday) + 1
	return t.AddDate(0, 0, diff)
}

func (s *server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user, ok := s.a.MustUser(w, r)
	if !ok {
		return
	}

	var teacherID uuid.UUID
	if user.Role == "Admin" {
		teacherIDRaw := r.URL.Query().Get("teacher_id")
		if teacherIDRaw == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "missing_teacher_id", "teacher_id query parameter required")
			return
		}
		parsed, err := uuid.Parse(teacherIDRaw)
		if err != nil {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_teacher_id", "Invalid teacher_id")
			return
		}
		teacherID = parsed
	} else if user.Role == "Teacher" {
		teacherID = user.ID
	} else {
		s.a.WriteErr(w, http.StatusForbidden, "forbidden", "Access denied")
		return
	}

	now := time.Now()

	var rangeStart, rangeEnd time.Time

	if monthStartRaw := r.URL.Query().Get("month_start"); monthStartRaw != "" {
		parsed, err := time.Parse("2006-01-02", monthStartRaw)
		if err == nil {
			rangeStart = time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.UTC)
			rangeEnd = rangeStart.AddDate(0, 1, 0)
		}
	}

	if rangeStart.IsZero() {
		rangeStart = getMonday(now)
		if weekStartRaw := r.URL.Query().Get("week_start"); weekStartRaw != "" {
			parsed, err := time.Parse("2006-01-02", weekStartRaw)
			if err == nil {
				rangeStart = getMonday(parsed)
			}
		}
		rangeEnd = rangeStart.AddDate(0, 0, 7)
	}

	rows, err := s.deps.Q.TeacherSessionsInRange(r.Context(), teacherID, rangeStart, rangeEnd)
	if err != nil {
		s.deps.Log.Error("failed to fetch teacher sessions", "error", err)
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Failed to load sessions")
		return
	}

	sessionIDs := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		sessionIDs = append(sessionIDs, row.ID)
	}

	absentStudentRows, err := s.deps.Q.AbsentStudentsBySessionIDs(r.Context(), sessionIDs)
	if err != nil {
		s.deps.Log.Error("failed to fetch absent students", "error", err)
	}

	sitInRows, err := s.deps.Q.SitInsBySessionIDs(r.Context(), sessionIDs)
	if err != nil {
		s.deps.Log.Error("failed to fetch sit-in data", "error", err)
	}

	absentStudentsBySession := make(map[string][]absentStudentDTO)
	for _, ar := range absentStudentRows {
		sid, err := s.a.UUIDString(ar.SessionID)
		if err != nil {
			continue
		}
		aid, err := s.a.UUIDString(ar.AbsenceID)
		if err != nil {
			continue
		}
		var name *string
		if ar.StudentName.Valid {
			name = &ar.StudentName.String
		}
		var nick *string
		if ar.Nickname.Valid {
			nick = &ar.Nickname.String
		}
		var createdAt *string
		if ar.CreatedAt.Valid {
			cts := ar.CreatedAt.Time.UTC().Format(time.RFC3339Nano)
			createdAt = &cts
		}
		absentStudentsBySession[sid] = append(absentStudentsBySession[sid], absentStudentDTO{
			Wcode:       ar.Wcode,
			Nickname:    nick,
			StudentName: name,
			AbsenceID:   aid,
			CreatedAt:   createdAt,
		})
	}

	sitInsBySession := make(map[string][]sitInVisitorDTO)
	for _, sr := range sitInRows {
		sid, err := s.a.UUIDString(sr.SessionID)
		if err != nil {
			continue
		}
		aid, err := s.a.UUIDString(sr.AbsenceID)
		if err != nil {
			continue
		}
		var name *string
		if sr.StudentName.Valid {
			name = &sr.StudentName.String
		}
		var nick *string
		if sr.Nickname.Valid {
			nick = &sr.Nickname.String
		}
		var fromSubjectName *string
		if sr.FromSubjectName.Valid {
			fromSubjectName = &sr.FromSubjectName.String
		}
		var absentSubjectName *string
		if sr.AbsentCourseSubjectName.Valid {
			absentSubjectName = &sr.AbsentCourseSubjectName.String
		}
		startS, _ := s.a.TimeString(sr.SessionStartAt)
		endS, _ := s.a.TimeString(sr.SessionEndAt)
		sitInsBySession[sid] = append(sitInsBySession[sid], sitInVisitorDTO{
			Wcode:             sr.Wcode,
			Nickname:          nick,
			StudentName:       name,
			FromCourseCode:    sr.FromCourseCode,
			FromSubjectName:   fromSubjectName,
			AbsenceID:         aid,
			SessionStartAt:    startS,
			SessionEndAt:      endS,
			AbsentSubjectName: absentSubjectName,
		})
	}

	var teacherUsername string
	sessions := make([]sessionDTO, 0, len(rows))
	totalAbsences := 0
	totalSitIns := 0

	for _, row := range rows {
		sid, err := s.a.UUIDString(row.ID)
		if err != nil {
			continue
		}
		cid, err := s.a.UUIDString(row.CourseID)
		if err != nil {
			continue
		}

		var subjectName *string
		if row.SubjectName.Valid {
			subjectName = &row.SubjectName.String
		}
		var roomName *string
		if row.RoomName.Valid {
			roomName = &row.RoomName.String
		}
		if row.TeacherName.Valid {
			teacherUsername = row.TeacherName.String
		}

		startS, _ := s.a.TimeString(row.StartAt)
		endS, _ := s.a.TimeString(row.EndAt)

		absStudents := absentStudentsBySession[sid]
		if absStudents == nil {
			absStudents = []absentStudentDTO{}
		}
		visitors := sitInsBySession[sid]
		if visitors == nil {
			visitors = []sitInVisitorDTO{}
		}

		absentCount := len(absStudents)
		totalAbsences += absentCount
		totalSitIns += len(visitors)

		sessions = append(sessions, sessionDTO{
			ID:            sid,
			CourseID:      cid,
			CourseCode:    row.CourseCode,
			CourseName:    row.CourseName,
			SubjectName:   subjectName,
			StartAt:       startS,
			EndAt:         endS,
			RoomName:      roomName,
			AbsentCount:   absentCount,
			AbsentStudents: absStudents,
			SitInVisitors: visitors,
		})
	}

	pendingRows, err := s.deps.Q.TeacherPendingAbsenceRequests(r.Context(), teacherID)
	if err != nil {
		s.deps.Log.Error("failed to fetch pending absence requests", "error", err)
	}

	pendingRequests := make([]pendingAbsenceRequestDTO, 0, len(pendingRows))
	for _, pr := range pendingRows {
		id, err := s.a.UUIDString(pr.ID)
		if err != nil {
			continue
		}
		var name *string
		if pr.StudentName.Valid {
			name = &pr.StudentName.String
		}
		var nick *string
		if pr.Nickname.Valid {
			nick = &pr.Nickname.String
		}
		var subjectName *string
		if pr.SubjectName.Valid {
			subjectName = &pr.SubjectName.String
		}
		var reason *string
		if pr.Reason.Valid {
			reason = &pr.Reason.String
		}
		var reasonCat *string
		if pr.ReasonCategory.Valid {
			reasonCat = &pr.ReasonCategory.String
		}
		pendingRequests = append(pendingRequests, pendingAbsenceRequestDTO{
			ID:             id,
			Wcode:          pr.Wcode,
			StudentName:    name,
			Nickname:       nick,
			CourseCode:     pr.CourseCode,
			CourseName:     pr.CourseName,
			SubjectName:    subjectName,
			DateFrom:       pr.DateFrom.Time.Format("2006-01-02"),
			DateTo:         pr.DateTo.Time.Format("2006-01-02"),
			Reason:         reason,
			ReasonCategory: reasonCat,
			CreatedAt:      pr.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
		})
	}

	teacherInfo := teacherInfoDTO{
		ID:       teacherID.String(),
		Username: teacherUsername,
	}
	if teacherUsername == "" {
		teacherInfo.Username = user.Username
	}

	resp := dashboardResponse{
		WeekStart:              rangeStart.Format("2006-01-02"),
		WeekEnd:                rangeEnd.Add(-24*time.Hour).Format("2006-01-02"),
		Teacher:                teacherInfo,
		Sessions:               sessions,
		Summary: weeklySummaryDTO{
			TotalSessions: len(sessions),
			TotalAbsences: totalAbsences,
			TotalSitIns:   totalSitIns,
		},
		PendingAbsenceRequests: pendingRequests,
	}

	s.a.WriteJSON(w, http.StatusOK, resp)
}
