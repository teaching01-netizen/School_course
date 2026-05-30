package absenceshttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/idempotency"
	"warwick-institute/internal/otp"
)

type parentVerificationSendRequest struct {
	Wcode string  `json:"wcode"`
	Token *string `json:"token"`
}

type parentVerificationVerifyRequest struct {
	Token string `json:"token"`
	Code  string `json:"code"`
}

type parentVerificationDTO struct {
	Token             string  `json:"token"`
	Status            string  `json:"status"`
	Wcode             string  `json:"wcode"`
	ParentPhone       *string `json:"parent_phone,omitempty"`
	OtpLastSentAt     *string `json:"otp_last_sent_at,omitempty"`
	OtpCodeExpiresAt  *string `json:"otp_code_expires_at,omitempty"`
	VerifiedAt        *string `json:"verified_at,omitempty"`
	ConsumedAt        *string `json:"consumed_at,omitempty"`
	ConsumedAbsenceID *string `json:"consumed_absence_id,omitempty"`
	ExpiresAt         *string `json:"expires_at,omitempty"`
}

const resendCooldown = 60 * time.Second

type publicRowExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func apiErr(code, message string) map[string]any {
	return map[string]any{"code": code, "message": message}
}

func (s *server) requestOriginAllowed(w http.ResponseWriter, r *http.Request) bool {
	if s.deps.AppOrigin == "" {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		origin = strings.TrimSpace(r.Header.Get("Referer"))
	}
	if origin == "" {
		s.a.WriteErr(w, http.StatusForbidden, "csrf_origin_required", "Origin or Referer header required")
		return false
	}
	allowed, err := url.Parse(s.deps.AppOrigin)
	if err != nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return false
	}
	got, err := url.Parse(origin)
	if err != nil {
		s.a.WriteErr(w, http.StatusForbidden, "csrf_origin_invalid", "Invalid Origin or Referer header")
		return false
	}
	if allowed.Scheme != got.Scheme || allowed.Host != got.Host {
		s.a.WriteErr(w, http.StatusForbidden, "csrf_origin_mismatch", "Invalid Origin or Referer header")
		return false
	}
	return true
}

func (s *server) requestIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *server) allowPublicRateLimit(ctx context.Context, key string, limit int, window time.Duration) (time.Duration, error) {
	if s.deps.RateLimiter == nil {
		return 0, nil
	}
	result, err := s.deps.RateLimiter.Allow(ctx, key, limit, window)
	if err != nil {
		return 0, err
	}
	if result.Allowed {
		return 0, nil
	}
	retryAfter := time.Until(result.ResetAt)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return retryAfter, nil
}

func (s *server) resolveAbsenceSelection(ctx context.Context, q *sqldb.Queries, db publicRowExecutor, wcode string, subjectIDRaw, courseIDRaw *string) (sqldb.Student, pgtype.UUID, sqldb.StudentEnrolledCourseV2, error) {
	student, err := q.StudentGetByWCode(ctx, wcode)
	if err != nil {
		return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
	}

	var subjectID pgtype.UUID
	var selectedCourse pgtype.UUID
	if subjectIDRaw != nil && strings.TrimSpace(*subjectIDRaw) != "" {
		subjectID, err = s.a.ParseUUID(strings.TrimSpace(*subjectIDRaw))
		if err != nil {
			return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
		}
	}

	if courseIDRaw != nil && strings.TrimSpace(*courseIDRaw) != "" {
		selectedCourse, err = s.a.ParseUUID(strings.TrimSpace(*courseIDRaw))
		if err != nil {
			return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
		}
		var courseRow struct {
			ID        pgtype.UUID
			Code      string
			Name      string
			SubjectID pgtype.UUID
		}
		if err := db.QueryRow(ctx, `SELECT id, code, name, subject_id FROM courses WHERE id = $1 AND deleted_at IS NULL`, selectedCourse).Scan(&courseRow.ID, &courseRow.Code, &courseRow.Name, &courseRow.SubjectID); err != nil {
			return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
		}
		if !subjectID.Valid {
			subjectID = courseRow.SubjectID
		} else if courseRow.SubjectID.Valid && courseRow.SubjectID != subjectID {
			return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, fmt.Errorf("course does not belong to selected subject")
		}
	}

	if !subjectID.Valid {
		return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, fmt.Errorf("subject_id or course_id is required")
	}

	enrolled, err := q.StudentEnrolledCoursesBySubjectV2(ctx, student.ID, subjectID)
	if err != nil || len(enrolled) == 0 {
		if err == nil {
			err = pgx.ErrNoRows
		}
		return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, err
	}

	chosen := enrolled[0]
	if selectedCourse.Valid {
		found := false
		for _, c := range enrolled {
			if c.CourseID == selectedCourse {
				chosen = c
				found = true
				break
			}
		}
		if !found {
			return sqldb.Student{}, pgtype.UUID{}, sqldb.StudentEnrolledCourseV2{}, fmt.Errorf("selected course is not enrolled for this student")
		}
	} else {
		for _, c := range enrolled {
			if c.Level.Valid && chosen.Level.Valid && c.Level.Int16 > chosen.Level.Int16 {
				chosen = c
			}
		}
	}

	return student, subjectID, chosen, nil
}

func verificationResponseFromState(row otp.SessionState, token string, expiresAt time.Time) parentVerificationDTO {
	dto := parentVerificationDTO{
		Token:  token,
		Status: row.Status,
		Wcode:  row.Wcode,
		ExpiresAt: func() *string {
			value := expiresAt.UTC().Format(time.RFC3339Nano)
			return &value
		}(),
	}
	phone := row.ParentPhone
	dto.ParentPhone = &phone
	if row.OTPLastSentAt.Valid {
		value := row.OTPLastSentAt.Time.UTC().Format(time.RFC3339Nano)
		dto.OtpLastSentAt = &value
	}
	if row.OTPCodeExpiresAt.Valid {
		value := row.OTPCodeExpiresAt.Time.UTC().Format(time.RFC3339Nano)
		dto.OtpCodeExpiresAt = &value
	}
	if row.VerifiedAt.Valid {
		value := row.VerifiedAt.Time.UTC().Format(time.RFC3339Nano)
		dto.VerifiedAt = &value
	}
	if row.ConsumedAt.Valid {
		value := row.ConsumedAt.Time.UTC().Format(time.RFC3339Nano)
		dto.ConsumedAt = &value
	}
	if row.ConsumedAbsence.Valid {
		if id, err := uuid.FromBytes(row.ConsumedAbsence.Bytes[:]); err == nil {
			value := id.String()
			dto.ConsumedAbsenceID = &value
		}
	}
	return dto
}

func (s *server) handleParentVerificationSend(w http.ResponseWriter, r *http.Request) {
	if !s.requestOriginAllowed(w, r) {
		return
	}
	if s.deps.OTP == nil || s.deps.OTPSender == nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	var body parentVerificationSendRequest
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}

	settings, err := s.readAbsenceSettings(r)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	if !settings.Notifications.SmsParentEnabled {
		s.a.WriteErr(w, http.StatusForbidden, "feature_disabled", "Parent verification codes are currently disabled")
		return
	}

	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "absences-public", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		nowKey := "parent-verification:wcode:"
		var phone string
		var code string
		var token string
		var info otp.TokenInfo

		if body.Token != nil && strings.TrimSpace(*body.Token) != "" {
			session, verifyErr := s.deps.OTP.LoadSessionTx(r.Context(), tx, strings.TrimSpace(*body.Token))
			if verifyErr != nil {
				switch {
				case errors.Is(verifyErr, otp.ErrExpired):
					s.a.WriteErr(w, http.StatusGone, "otp_expired", "Verification code expired")
				case errors.Is(verifyErr, otp.ErrTampered):
					s.a.WriteErr(w, http.StatusBadRequest, "bad_token", "Verification token is invalid")
				default:
					status, code, msg := s.a.ClassifyDBErr(verifyErr)
					s.a.WriteErr(w, status, code, msg)
				}
				return 0, nil, verifyErr
			}
			info, _ = s.deps.OTP.DecodeToken(strings.TrimSpace(*body.Token))
			phone = session.ParentPhone
			if retryAfter, err := s.allowPublicRateLimit(r.Context(), nowKey+session.Wcode, 20, time.Hour); err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			} else if retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				s.a.WriteErr(w, http.StatusTooManyRequests, "rate_limited", "Too many verification requests")
				return 0, nil, fmt.Errorf("rate limited")
			}
			if retryAfter, err := s.allowPublicRateLimit(r.Context(), "parent-verification:ip:"+s.requestIP(r), 50, time.Hour); err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			} else if retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				s.a.WriteErr(w, http.StatusTooManyRequests, "rate_limited", "Too many verification requests")
				return 0, nil, fmt.Errorf("rate limited")
			}
			if allowed, retryAfter, err := s.deps.CircuitBreaker.Allow(r.Context()); err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			} else if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				s.a.WriteErr(w, http.StatusServiceUnavailable, "sms_circuit_open", "Verification service is temporarily unavailable")
				return 0, nil, fmt.Errorf("circuit open")
			}

			code, token, err = s.deps.OTP.ResendSessionTx(r.Context(), tx, strings.TrimSpace(*body.Token))
			if err != nil {
				switch {
				case errors.Is(err, otp.ErrCooldown):
					w.Header().Set("Retry-After", strconv.Itoa(int(resendCooldown.Seconds())))
					s.a.WriteErr(w, http.StatusTooManyRequests, "otp_cooldown", "Please wait before requesting another code")
				case errors.Is(err, otp.ErrLocked):
					s.a.WriteErr(w, http.StatusLocked, "otp_locked", "Too many verification attempts")
				case errors.Is(err, otp.ErrStudentLocked):
					s.a.WriteErr(w, http.StatusLocked, "student_locked", "This student is temporarily locked")
				case errors.Is(err, otp.ErrAlreadyVerified):
					s.a.WriteErr(w, http.StatusConflict, "already_verified", "This verification session is already complete")
				case errors.Is(err, otp.ErrInvalidPhone):
					s.a.WriteErr(w, http.StatusConflict, "parent_phone_missing", "No parent phone is on file for this student")
				default:
					status, code, msg := s.a.ClassifyDBErr(err)
					s.a.WriteErr(w, status, code, msg)
				}
				return 0, nil, err
			}
			_ = phone
			if err := s.deps.OTPSender.SendOTP(r.Context(), phone, code); err != nil {
				if s.deps.Log != nil {
					s.deps.Log.Error("otp sms resend failed", "phone", phone, "error", err)
				}
				if retryAfter, cbErr := s.deps.CircuitBreaker.ReportFailure(r.Context()); cbErr == nil && retryAfter > 0 {
					w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				}
				s.a.WriteErr(w, http.StatusBadGateway, "sms_send_failed", "Could not send verification code")
				return 0, nil, err
			}
			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := s.deps.CircuitBreaker.ReportSuccess(bgCtx); err != nil && s.deps.Log != nil {
					s.deps.Log.Warn("circuit breaker success update failed", "error", err)
				}
			}()

			info, _ = s.deps.OTP.DecodeToken(token)
			session, err := s.deps.OTP.LoadSessionTx(r.Context(), tx, strings.TrimSpace(*body.Token))
			if err != nil {
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
				return 0, nil, err
			}
			resp := verificationResponseFromState(session, token, info.ExpiresAt)
			return http.StatusOK, resp, nil
		}

		if strings.TrimSpace(body.Wcode) == "" {
			s.a.WriteErr(w, http.StatusBadRequest, "bad_wcode", "wcode is required")
			return 0, nil, fmt.Errorf("wcode required")
		}

		if retryAfter, err := s.allowPublicRateLimit(r.Context(), nowKey+strings.TrimSpace(body.Wcode), 20, time.Hour); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		} else if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			s.a.WriteErr(w, http.StatusTooManyRequests, "rate_limited", "Too many verification requests")
			return 0, nil, fmt.Errorf("rate limited")
		}
		if retryAfter, err := s.allowPublicRateLimit(r.Context(), "parent-verification:ip:"+s.requestIP(r), 50, time.Hour); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		} else if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			s.a.WriteErr(w, http.StatusTooManyRequests, "rate_limited", "Too many verification requests")
			return 0, nil, fmt.Errorf("rate limited")
		}
		if allowed, retryAfter, err := s.deps.CircuitBreaker.Allow(r.Context()); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		} else if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			s.a.WriteErr(w, http.StatusServiceUnavailable, "sms_circuit_open", "Verification service is temporarily unavailable")
			return 0, nil, fmt.Errorf("circuit open")
		}

		rows, err := s.deps.Q.StudentSubjectByWCode(r.Context(), strings.TrimSpace(body.Wcode))
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if len(rows) == 0 {
			s.a.WriteErr(w, http.StatusNotFound, "no_subjects", "No enrolled subjects found for this student")
			return 0, nil, fmt.Errorf("no subjects")
		}
		if !rows[0].ParentPhone.Valid || strings.TrimSpace(rows[0].ParentPhone.String) == "" {
			s.a.WriteErr(w, http.StatusConflict, "parent_phone_missing", "No parent phone is on file for this student")
			return 0, nil, fmt.Errorf("parent phone missing")
		}

		code, token, err = s.deps.OTP.StartSessionTx(r.Context(), tx, strings.TrimSpace(body.Wcode), rows[0].ParentPhone.String)
		if err != nil {
			switch {
			case errors.Is(err, otp.ErrLocked):
				s.a.WriteErr(w, http.StatusLocked, "otp_locked", "Too many verification attempts")
			case errors.Is(err, otp.ErrStudentLocked):
				s.a.WriteErr(w, http.StatusLocked, "student_locked", "This student is temporarily locked")
			case errors.Is(err, otp.ErrInvalidPhone):
				s.a.WriteErr(w, http.StatusConflict, "parent_phone_missing", "No parent phone is on file for this student")
			default:
				status, code, msg := s.a.ClassifyDBErr(err)
				s.a.WriteErr(w, status, code, msg)
			}
			return 0, nil, err
		}
		if err := s.deps.OTPSender.SendOTP(r.Context(), rows[0].ParentPhone.String, code); err != nil {
			if s.deps.Log != nil {
				s.deps.Log.Error("otp sms send failed", "phone", rows[0].ParentPhone.String, "error", err)
			}
			if retryAfter, cbErr := s.deps.CircuitBreaker.ReportFailure(r.Context()); cbErr == nil && retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			}
			s.a.WriteErr(w, http.StatusBadGateway, "sms_send_failed", "Could not send verification code")
			return 0, nil, err
		}
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.deps.CircuitBreaker.ReportSuccess(bgCtx); err != nil && s.deps.Log != nil {
				s.deps.Log.Warn("circuit breaker success update failed", "error", err)
			}
		}()

		info, _ = s.deps.OTP.DecodeToken(token)
		session, err := s.deps.OTP.LoadSessionTx(r.Context(), tx, token)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		resp := verificationResponseFromState(session, token, info.ExpiresAt)
		return http.StatusOK, resp, nil
	}) {
		return
	}
}

func (s *server) handleParentVerificationVerify(w http.ResponseWriter, r *http.Request) {
	if !s.requestOriginAllowed(w, r) {
		return
	}
	if s.deps.OTP == nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}

	var body parentVerificationVerifyRequest
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_json", "Invalid JSON")
		return
	}
	if strings.TrimSpace(body.Token) == "" || strings.TrimSpace(body.Code) == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_code", "token and code are required")
		return
	}

	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "absences-public", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		session, verifyErr := s.deps.OTP.VerifySessionTx(r.Context(), tx, strings.TrimSpace(body.Token), strings.TrimSpace(body.Code))
		switch {
		case verifyErr == nil || errors.Is(verifyErr, otp.ErrAlreadyVerified):
			info, _ := s.deps.OTP.DecodeToken(strings.TrimSpace(body.Token))
			resp := verificationResponseFromState(session, body.Token, info.ExpiresAt)
			return http.StatusOK, resp, nil
		case errors.Is(verifyErr, otp.ErrInvalid), errors.Is(verifyErr, otp.ErrLocked), errors.Is(verifyErr, otp.ErrStudentLocked):
			status := http.StatusBadRequest
			code := "invalid_code"
			message := "Invalid verification code"
			if errors.Is(verifyErr, otp.ErrLocked) {
				status = http.StatusLocked
				code = "otp_locked"
				message = "Too many verification attempts"
			}
			if errors.Is(verifyErr, otp.ErrStudentLocked) {
				status = http.StatusLocked
				code = "student_locked"
				message = "This student is temporarily locked"
			}
			return status, apiErr(code, message), nil
		case errors.Is(verifyErr, otp.ErrExpired), errors.Is(verifyErr, otp.ErrTampered), errors.Is(verifyErr, otp.ErrSuperseded):
			status := http.StatusBadRequest
			code := "bad_code"
			message := "Verification code is no longer valid"
			if errors.Is(verifyErr, otp.ErrExpired) {
				status = http.StatusGone
				code = "otp_expired"
				message = "Verification code expired"
			} else if errors.Is(verifyErr, otp.ErrTampered) {
				code = "bad_token"
				message = "Verification token is invalid"
			} else if errors.Is(verifyErr, otp.ErrSuperseded) {
				status = http.StatusConflict
				code = "otp_superseded"
				message = "A newer verification code has already been sent"
			}
			return status, apiErr(code, message), nil
		default:
			status, code, msg := s.a.ClassifyDBErr(verifyErr)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, verifyErr
		}
	}) {
		return
	}
}

func (s *server) handleParentVerificationGet(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_token", "token is required")
		return
	}
	info, err := s.deps.OTP.DecodeToken(token)
	if err != nil {
		status := http.StatusBadRequest
		code := "bad_token"
		message := "Invalid verification token"
		if errors.Is(err, otp.ErrExpired) {
			status = http.StatusGone
			code = "otp_expired"
			message = "Verification token expired"
		}
		s.a.WriteErr(w, status, code, message)
		return
	}
	session, err := s.deps.OTP.LoadSession(r.Context(), token)
	if err != nil {
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
		return
	}
	resp := verificationResponseFromState(session, token, info.ExpiresAt)
	s.a.WriteJSON(w, http.StatusOK, resp)
}

func (s *server) handlePendingCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requestOriginAllowed(w, r) {
		return
	}

	id, err := s.a.ParseUUID(r.PathValue("id"))
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_id", "Invalid id")
		return
	}

	if !s.a.WithIdempotentTx(w, r, idempotency.SystemActorUUID, "absences-public", s.deps.DB, s.deps.Q, func(tx pgx.Tx) (int, any, error) {
		qtx := s.deps.Q.WithTx(tx)
		row, err := qtx.ManagedAbsenceGet(r.Context(), id)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if row.Status == "cancelled" {
			return http.StatusOK, managedAbsenceResponse(row), nil
		}
		if row.Status != "pending" && row.Status != "reviewed" {
			s.a.WriteErr(w, http.StatusConflict, "bad_status", "This absence cannot be cancelled")
			return 0, nil, fmt.Errorf("bad status")
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE student_absences
			SET status = 'cancelled',
			    updated_at = now(),
			    version = version + 1
			WHERE id = $1
		`, id); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		if err := qtx.AbsenceAuditInsert(r.Context(), sqldb.AbsenceAuditInsertParams{
			AbsenceID: id,
			Action:    "cancelled",
			ActorRole: "student",
			Details:   map[string]any{"wcode": row.Wcode},
		}); err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		row, err = qtx.ManagedAbsenceGet(r.Context(), id)
		if err != nil {
			status, code, msg := s.a.ClassifyDBErr(err)
			s.a.WriteErr(w, status, code, msg)
			return 0, nil, err
		}
		return http.StatusOK, managedAbsenceResponse(row), nil
	}) {
		return
	}
}
