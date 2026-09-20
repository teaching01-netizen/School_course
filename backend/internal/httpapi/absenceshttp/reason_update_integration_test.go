package absenceshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

func TestAbsenceReasonUpdate_PersistsVersionAndFullAuditHistory(t *testing.T) {
	databaseURL := requireTestDBMgmt(t)
	migrateUpOnceMgmt(t, databaseURL)
	dbpool := newPoolMgmt(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx := context.Background()
	q := sqldb.New(dbpool)
	suffix := uuid.NewString()[:8]
	adminID := uuid.New()
	wcode := "REASON" + suffix

	if _, err := dbpool.Exec(ctx, `INSERT INTO users (id, username, role, password_hash) VALUES ($1, $2, 'Admin', 'test')`, adminID, "reason-admin-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, 'Reason Test Student')`, wcode); err != nil {
		t.Fatal(err)
	}
	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{Code: "REASON-SUBJ-" + suffix, Name: "Reason Subject"})
	if err != nil {
		t.Fatal(err)
	}
	var courseID, absenceID pgtype.UUID
	if err := dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id) VALUES ($1, 'Reason Course', $2) RETURNING id`, "REASON-COURSE-"+suffix, subject.ID).Scan(&courseID); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO student_absences (wcode, course_id, subject_id, date_from, date_to, status, reason)
		VALUES ($1, $2, $3, '2026-09-01', '2026-09-01', 'pending', 'Original reason')
		RETURNING id
	`, wcode, courseID, subject.ID).Scan(&absenceID); err != nil {
		t.Fatal(err)
	}
	absenceIDString, err := uuidString(absenceID)
	if err != nil {
		t.Fatal(err)
	}

	authService := statusTransitionAuth{user: auth.AuthenticatedUser{ID: adminID, Role: "Admin"}}
	deps := httpdeps.Deps{Auth: authService, Q: q, DB: dbpool, InstituteTZ: "Asia/Bangkok"}
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	call := func(key, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/absences/"+absenceIDString+"/reason", bytes.NewBufferString(body))
		req.SetPathValue("id", absenceIDString)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		s.handleAbsenceReasonUpdate(w, req)
		return w
	}

	first := call("reason-update-1-"+suffix, `{"reason":"  Updated reason  ","expected_version":1}`)
	if first.Code != http.StatusOK {
		t.Fatalf("reason update status = %d, body = %s", first.Code, first.Body.String())
	}
	var firstResponse map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &firstResponse); err != nil {
		t.Fatal(err)
	}
	if firstResponse["reason"] != "Updated reason" || firstResponse["version"] != float64(2) {
		t.Fatalf("unexpected first response: %#v", firstResponse)
	}

	var reason pgtype.Text
	var version int32
	if err := dbpool.QueryRow(ctx, `SELECT reason, version FROM student_absences WHERE id = $1`, absenceID).Scan(&reason, &version); err != nil {
		t.Fatal(err)
	}
	if !reason.Valid || reason.String != "Updated reason" || version != 2 {
		t.Fatalf("stored reason/version = %#v/%d, want Updated reason/2", reason, version)
	}

	var action string
	var details []byte
	if err := dbpool.QueryRow(ctx, `SELECT action, details FROM absence_audit_log WHERE absence_id = $1 ORDER BY created_at DESC LIMIT 1`, absenceID).Scan(&action, &details); err != nil {
		t.Fatal(err)
	}
	if action != "reason_updated" {
		t.Fatalf("audit action = %q, want reason_updated", action)
	}
	var auditDetails map[string]any
	if err := json.Unmarshal(details, &auditDetails); err != nil {
		t.Fatal(err)
	}
	if auditDetails["previous_reason"] != "Original reason" || auditDetails["new_reason"] != "Updated reason" {
		t.Fatalf("audit details = %#v", auditDetails)
	}
	var auditAction string
	var auditPayload []byte
	if err := dbpool.QueryRow(ctx, `SELECT action, payload FROM audit_log WHERE action = 'absence.reason_updated' ORDER BY id DESC LIMIT 1`).Scan(&auditAction, &auditPayload); err != nil {
		t.Fatal(err)
	}
	if auditAction != "absence.reason_updated" || strings.Contains(string(auditPayload), "Updated reason") || strings.Contains(string(auditPayload), "Original reason") {
		t.Fatalf("sensitive text leaked into audit log: action=%q payload=%s", auditAction, auditPayload)
	}

	cleared := call("reason-update-clear-"+suffix, `{"reason":"   ","expected_version":2}`)
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear reason update status = %d, body = %s", cleared.Code, cleared.Body.String())
	}
	var clearResponse map[string]any
	if err := json.Unmarshal(cleared.Body.Bytes(), &clearResponse); err != nil {
		t.Fatal(err)
	}
	if clearResponse["reason"] != nil || clearResponse["version"] != float64(3) {
		t.Fatalf("unexpected clear response: %#v", clearResponse)
	}
	if err := dbpool.QueryRow(ctx, `SELECT reason, version FROM student_absences WHERE id = $1`, absenceID).Scan(&reason, &version); err != nil {
		t.Fatal(err)
	}
	if reason.Valid || version != 3 {
		t.Fatalf("cleared reason/version = %#v/%d, want NULL/3", reason, version)
	}

	stale := call("reason-update-stale-"+suffix, `{"reason":"Should not persist","expected_version":1}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale reason update status = %d, body = %s", stale.Code, stale.Body.String())
	}
	if err := dbpool.QueryRow(ctx, `SELECT reason, version FROM student_absences WHERE id = $1`, absenceID).Scan(&reason, &version); err != nil {
		t.Fatal(err)
	}
	if reason.Valid || version != 3 {
		t.Fatalf("stale update changed reason/version to %#v/%d", reason, version)
	}
	var timelineCount int
	if err := dbpool.QueryRow(ctx, `SELECT count(*) FROM absence_audit_log WHERE absence_id = $1 AND action = 'reason_updated'`, absenceID).Scan(&timelineCount); err != nil {
		t.Fatal(err)
	}
	if timelineCount != 2 {
		t.Fatalf("stale update wrote an audit entry; reason_updated count = %d", timelineCount)
	}
}

func TestAbsenceReasonUpdate_RejectsTeacher(t *testing.T) {
	authService := statusTransitionAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Role: "Teacher"}}
	deps := httpdeps.Deps{Auth: authService}
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}
	id := uuid.NewString()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/absences/"+id+"/reason", bytes.NewBufferString(`{"reason":"Nope","expected_version":1}`))
	req.SetPathValue("id", id)
	req.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	s.handleAbsenceReasonUpdate(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("teacher reason update status = %d, body = %s", w.Code, w.Body.String())
	}
}
