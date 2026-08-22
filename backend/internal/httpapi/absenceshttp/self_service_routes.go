package absenceshttp

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"warwick-institute/internal/absences/selfservice"
	"warwick-institute/internal/studentauth"
)

func (s *server) studentAuthService() *studentauth.Service {
	if s.deps.StudentSelfService != nil {
		return s.deps.StudentSelfService
	}
	if s.deps.DB == nil {
		return nil
	}
	return studentauth.NewService(s.deps.DB)
}

func (s *server) requireStudentSession(w http.ResponseWriter, r *http.Request) (studentauth.Session, bool) {
	service := s.studentAuthService()
	if service == nil {
		s.a.WriteErr(w, http.StatusUnauthorized, "unauthorized", "Student verification is required")
		return studentauth.Session{}, false
	}
	rawToken := studentauth.ReadSessionCookie(r, s.deps.StudentCookieSecure)
	session, err := service.ValidateSession(r.Context(), rawToken)
	if err != nil {
		s.a.WriteErr(w, http.StatusUnauthorized, "unauthorized", "Student verification is required")
		return studentauth.Session{}, false
	}
	return session, true
}

func (s *server) handleStudentProfile(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireStudentSession(w, r)
	if !ok {
		return
	}
	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if !settings.StudentSelfService.CanViewOwn {
		s.a.WriteErr(w, http.StatusForbidden, "feature_disabled", "Student profile access is currently disabled")
		return
	}

	rows, err := s.deps.Q.StudentSubjectByWCode(r.Context(), session.Wcode)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if len(rows) == 0 {
		student, studentErr := s.deps.Q.StudentGetByWCode(r.Context(), session.Wcode)
		if errors.Is(studentErr, pgx.ErrNoRows) {
			s.a.WriteErr(w, http.StatusNotFound, "student_not_found", "Student profile was not found")
			return
		}
		if studentErr != nil {
			status, code, msg := s.a.ClassifyDBErr(studentErr)
			s.a.WriteErr(w, status, code, msg)
			return
		}
		// The profile reports whether a nickname exists, never the value, so
		// the public form can offer to fill a missing one without a leak.
		nicknameSet := student.Nickname.Valid && strings.TrimSpace(student.Nickname.String) != ""
		s.a.WriteJSON(w, http.StatusOK, map[string]any{
			"wcode":         student.Wcode,
			"display_name":  student.FullName,
			"email_on_file": false,
			"nickname_set":  nicknameSet,
			"subjects":      []any{},
		})
		return
	}

	type subject struct {
		ID             string `json:"id"`
		Code           string `json:"code"`
		Name           string `json:"name"`
		TeacherName    string `json:"teacher_name,omitempty"`
		MergeGroupID   string `json:"merge_group_id,omitempty"`
		MergeGroupName string `json:"merge_group_name,omitempty"`
	}
	subjects := make([]subject, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	displayName := rows[0].FullName
	emailOnFile := false
	for _, row := range rows {
		if row.Nickname.Valid && strings.TrimSpace(row.Nickname.String) != "" {
			displayName = row.Nickname.String
		}
		if row.EmailCRM.Valid && strings.TrimSpace(row.EmailCRM.String) != "" || row.EmailSystem.Valid && strings.TrimSpace(row.EmailSystem.String) != "" || row.Email.Valid && strings.TrimSpace(row.Email.String) != "" {
			emailOnFile = true
		}
		id, idErr := s.a.UUIDString(row.SubjectID)
		if idErr != nil {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		item := subject{ID: id, Code: row.SubjectCode, Name: row.SubjectName, TeacherName: row.ActiveTeacherName}
		if row.MergeGroupID.Valid {
			if mergeID, mergeErr := s.a.UUIDString(row.MergeGroupID); mergeErr == nil {
				item.MergeGroupID = mergeID
				item.MergeGroupName = row.MergeGroupName.String
			}
		}
		subjects = append(subjects, item)
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{
		"wcode":         session.Wcode,
		"display_name":  displayName,
		"email_on_file": emailOnFile,
		"nickname_set":  rows[0].Nickname.Valid && strings.TrimSpace(rows[0].Nickname.String) != "",
		"subjects":      subjects,
	})
}

func (s *server) handleStudentSessions(w http.ResponseWriter, r *http.Request) {
	if _, supplied := r.URL.Query()["wcode"]; supplied {
		s.a.WriteErr(w, http.StatusBadRequest, "identity_parameter_not_allowed", "wcode is derived from the verified student session")
		return
	}
	if _, supplied := r.URL.Query()["bypass_timing"]; supplied {
		s.a.WriteErr(w, http.StatusBadRequest, "bypass_not_allowed", "bypass_timing is not available to students")
		return
	}
	if _, supplied := r.URL.Query()["include_all_subjects"]; supplied {
		s.a.WriteErr(w, http.StatusBadRequest, "include_all_subjects_not_allowed", "include_all_subjects is not available to students")
		return
	}
	studentSession, ok := s.requireStudentSession(w, r)
	if !ok {
		return
	}
	s.handleSessionsInRangeForWCode(w, r, studentSession.Wcode, false)
}

func (s *server) handleStudentAbsenceHistory(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireStudentSession(w, r)
	if !ok {
		return
	}
	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if !settings.StudentSelfService.CanViewOwn {
		s.a.WriteErr(w, http.StatusForbidden, "feature_disabled", "Student absence history is currently disabled")
		return
	}
	items, err := selfservice.ListOwnHistory(r.Context(), s.deps.DB, session.Wcode)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	s.a.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) handleStudentAbsenceCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requestOriginAllowed(w, r) {
		return
	}
	session, ok := s.requireStudentSession(w, r)
	if !ok {
		return
	}
	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}
	if !s.a.WithIdempotentTx(w, r, studentIdempotencyActor(session.Wcode), "absences-public", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		settings, settingsErr := s.readAbsenceSettings(r)
		if settingsErr != nil {
			status, code, msg := s.a.ClassifyDBErr(settingsErr)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, settingsErr
		}
		row, cancelErr := selfservice.CancelOwn(r.Context(), tx, s.deps.Q.WithTx(tx), selfservice.CancelRequest{
			AbsenceID:    id,
			Wcode:        session.Wcode,
			CanCancelOwn: settings.StudentSelfService.CanCancelOwn,
		})
		if cancelErr != nil {
			switch {
			case errors.Is(cancelErr, selfservice.ErrAbsenceNotFound):
				s.a.WriteErr(w, http.StatusNotFound, "not_found", "Absence not found")
			case errors.Is(cancelErr, selfservice.ErrCancellationDisabled):
				s.a.WriteErr(w, http.StatusForbidden, "cancellation_disabled", "Student absence cancellation is disabled")
			case errors.Is(cancelErr, selfservice.ErrNotCancellable):
				s.a.WriteErr(w, http.StatusConflict, "bad_status", "This absence cannot be cancelled")
			default:
				status, code, msg := s.a.ClassifyDBErr(cancelErr)
				s.a.WriteErr(w, status, code, msg)
			}
			return 0, nil, cancelErr
		}
		return http.StatusOK, studentAbsenceResponse(row), nil
	}) {
		return
	}
	s.publishAbsenceChanged(id.String())
}
