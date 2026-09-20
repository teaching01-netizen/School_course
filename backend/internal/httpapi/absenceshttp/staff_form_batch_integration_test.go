package absenceshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"warwick-institute/internal/auth"
	"warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
)

func TestStaffAbsenceFormBatch_TeacherCreatesPendingWithoutNotificationSideEffects(t *testing.T) {
	databaseURL := requireStaffTestDB(t)
	migrateStaffUpOnce(t, databaseURL)
	dbpool := newStaffPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := db.New(dbpool)
	studentWcode, subjectID, courseID, sessionID, _ := seedStaffCreateData(t, q, dbpool, "FORM")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	teacherIDPG, err := q.AdminUserCreate(ctx, db.AdminUserCreateParams{
		Username:     "form-teacher-" + uuid.NewString(),
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	teacherID, err := uuid.FromBytes(teacherIDPG.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}

	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	sessionStart := now.Add(48 * time.Hour)
	if _, err := dbpool.Exec(ctx, "UPDATE sessions SET start_at = $1, end_at = $2 WHERE id = $3", sessionStart, sessionStart.Add(90*time.Minute), sessionID); err != nil {
		t.Fatal(err)
	}
	sessionDate := sessionStart.In(loc).Format("2006-01-02")

	fa := staffFakeAuth{user: auth.AuthenticatedUser{ID: teacherID, Username: "teacher", Role: "Teacher"}}
	deps := httpdeps.Deps{
		Log:         slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Auth:        fa,
		Q:           q,
		DB:          dbpool,
		InstituteTZ: "Asia/Bangkok",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := http.NewServeMux()
		Register(mux, deps)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	body := map[string]any{
		"items": []map[string]any{{
			"wcode":              studentWcode,
			"subject_id":         subjectID,
			"course_id":          courseID,
			"date_from":          sessionDate,
			"date_to":            sessionDate,
			"missed_session_ids": []string{sessionID},
			"sit_in_session_ids": []string{},
			"reason":             "Medical appointment",
			"status":             "pending",
		}},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	doRequest := func() (int, []byte) {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/absences/staff-form-batch", bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "staff-form-idempotency-test")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, responseBody
	}

	status, firstBody := doRequest()
	if status != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", status, firstBody)
	}
	status, replayBody := doRequest()
	if status != http.StatusCreated {
		t.Fatalf("idempotent replay expected 201 Created, got %d: %s", status, replayBody)
	}
	if !bytes.Equal(firstBody, replayBody) {
		t.Fatalf("idempotent replay changed response: first=%s replay=%s", firstBody, replayBody)
	}

	var response struct {
		IDs   []string `json:"ids"`
		Items []struct {
			ID         string          `json:"id"`
			Status     string          `json:"status"`
			SmsPreview json.RawMessage `json:"sms_preview"`
		} `json:"items"`
	}
	if err := json.Unmarshal(firstBody, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.IDs) != 1 || len(response.Items) != 1 {
		t.Fatalf("unexpected response shape: %+v", response)
	}
	if response.Items[0].Status != "pending" {
		t.Fatalf("staff form status = %q, want pending", response.Items[0].Status)
	}
	if len(response.Items[0].SmsPreview) != 0 && string(response.Items[0].SmsPreview) != "null" {
		t.Fatalf("staff form response included SMS preview: %s", response.Items[0].SmsPreview)
	}

	var storedStatus string
	if err := dbpool.QueryRow(ctx, "SELECT status FROM student_absences WHERE id = $1", response.IDs[0]).Scan(&storedStatus); err != nil {
		t.Fatal(err)
	}
	if storedStatus != "pending" {
		t.Fatalf("stored staff form status = %q, want pending", storedStatus)
	}

	var actorID uuid.UUID
	var action string
	if err := dbpool.QueryRow(ctx, "SELECT actor_id, action FROM absence_audit_log WHERE absence_id = $1 ORDER BY created_at DESC LIMIT 1", response.IDs[0]).Scan(&actorID, &action); err != nil {
		t.Fatal(err)
	}
	if actorID != teacherID || action != "created_by_staff" {
		t.Fatalf("audit actor/action = %s/%q, want %s/created_by_staff", actorID, action, teacherID)
	}

	var notificationCount int
	if err := dbpool.QueryRow(ctx, "SELECT count(*) FROM notification_outbox WHERE absence_id = $1", response.IDs[0]).Scan(&notificationCount); err != nil {
		t.Fatal(err)
	}
	if notificationCount != 0 {
		t.Fatalf("staff form created %d notification outbox rows", notificationCount)
	}
}
