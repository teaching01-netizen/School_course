package absenceshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/smartsms"
)

type statusTransitionSMSRecorder struct {
	sent int
}

func (r *statusTransitionSMSRecorder) SendSMS(_ context.Context, _ smartsms.SendRequest) (*smartsms.SendResponse, error) {
	r.sent++
	return &smartsms.SendResponse{Success: true}, nil
}

func (r *statusTransitionSMSRecorder) HealthCheck(_ context.Context) error { return nil }
func (r *statusTransitionSMSRecorder) GetCredits(_ context.Context) (int, error) {
	return 999, nil
}

type statusTransitionAuth struct {
	user auth.AuthenticatedUser
}

func (a statusTransitionAuth) RequireUser(context.Context, *http.Request) (auth.AuthenticatedUser, error) {
	return a.user, nil
}

func (statusTransitionAuth) HandleLogin(http.ResponseWriter, *http.Request) error  { return nil }
func (statusTransitionAuth) HandleLogout(http.ResponseWriter, *http.Request) error { return nil }

func TestAbsenceStatusUpdate_DoesNotSendAutomaticSuccessSMS(t *testing.T) {
	databaseURL := requireTestDBMgmt(t)
	migrateUpOnceMgmt(t, databaseURL)
	dbpool := newPoolMgmt(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx := context.Background()
	q := sqldb.New(dbpool)
	suffix := uuid.NewString()[:8]
	wcode := "STATUS" + suffix
	adminID := uuid.New()

	_, err := dbpool.Exec(ctx, `
		INSERT INTO users (id, username, role, password_hash)
		VALUES ($1, $2, 'Admin', 'test')
	`, adminID, "status-admin-"+suffix)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dbpool.Exec(ctx, `
		INSERT INTO students (wcode, full_name, parent_phone, student_phone)
		VALUES ($1, 'Status Test Student', '+66810000001', '+66810000002')
	`, wcode)
	if err != nil {
		t.Fatal(err)
	}

	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: "STATUS-SUBJ-" + suffix,
		Name: "Status Test Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	var courseID pgtype.UUID
	err = dbpool.QueryRow(ctx, `
		INSERT INTO courses (code, name, subject_id)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "STATUS-COURSE-"+suffix, "Status Test Course "+suffix, subject.ID).Scan(&courseID)
	if err != nil {
		t.Fatal(err)
	}

	var absenceID pgtype.UUID
	err = dbpool.QueryRow(ctx, `
		INSERT INTO student_absences (
			wcode, course_id, subject_id, date_from, date_to, status, student_name, student_phone, reason
		)
		VALUES ($1, $2, $3, '2026-08-20', '2026-08-20', 'pending', 'Status Test Student', '+66810000002', 'Test')
		RETURNING id
	`, wcode, courseID, subject.ID).Scan(&absenceID)
	if err != nil {
		t.Fatal(err)
	}

	absenceIDString, err := uuidString(absenceID)
	if err != nil {
		t.Fatal(err)
	}

	provider := &statusTransitionSMSRecorder{}
	deps := httpdeps.Deps{
		Auth:        statusTransitionAuth{user: auth.AuthenticatedUser{ID: adminID, Role: "Admin"}},
		Q:           q,
		DB:          dbpool,
		SMS:         provider,
		Log:         slog.Default(),
		InstituteTZ: "UTC",
	}
	s := &server{deps: deps, a: httpadapter.New(deps.Auth, deps.Log)}

	for index, status := range []string{"reviewed", "actioned"} {
		expectedVersion := index + 1
		body := fmt.Sprintf(`{"status":%q,"expected_version":%d}`, status, expectedVersion)
		req := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/absences/"+absenceIDString+"/status",
			bytes.NewBufferString(body),
		)
		req.SetPathValue("id", absenceIDString)
		req.Header.Set("Idempotency-Key", "status-"+status+"-"+suffix)
		w := httptest.NewRecorder()

		s.handleAbsenceStatusUpdate(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status update = %d, body = %s", status, w.Code, w.Body.String())
		}
	}

	if provider.sent != 0 {
		t.Fatalf("automatic status notifications sent %d SMS messages, want 0", provider.sent)
	}
}

func TestBatchSendSuccessSMS_PreservesMergedCourseMakeUps(t *testing.T) {
	databaseURL := requireStaffTestDB(t)
	migrateStaffUpOnce(t, databaseURL)
	pool := newStaffPool(t, databaseURL)
	t.Cleanup(pool.Close)
	q := sqldb.New(pool)
	wcode, subjectID, courseID, _, adminID := seedStaffCreateData(t, q, pool, "SMS")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	const phone = "+66810000001"
	if _, err := pool.Exec(ctx, "UPDATE students SET parent_phone = $1 WHERE wcode = $2", phone, wcode); err != nil {
		t.Fatal(err)
	}

	mergeName := "Merged SAT Verbal " + wcode
	var mergeID pgtype.UUID
	if err := pool.QueryRow(ctx, "INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id", mergeName).Scan(&mergeID); err != nil {
		t.Fatal(err)
	}
	var ids []uuid.UUID
	for _, fixture := range []struct {
		absenceDate string
		targetName  string
		startAt     string
		endAt       string
	}{
		{"2026-11-28", "Rank 3 Section 1", "2026-12-02T17:00:00+07:00", "2026-12-02T18:40:00+07:00"},
		{"2026-11-29", "Rank 4", "2026-12-01T17:00:00+07:00", "2026-12-01T20:20:00+07:00"},
	} {
		target, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: wcode + fixture.targetName, Name: fixture.targetName})
		if err != nil {
			t.Fatal(err)
		}
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `
			INSERT INTO student_absences (wcode, subject_id, course_id, merge_group_id, date_from, date_to, status, sit_in_method, sit_in_course_id)
			VALUES ($1, $2, $3, $4, $5, $5, 'special_approved', 'physical', $6)
			RETURNING id
		`, wcode, subjectID, courseID, mergeID, fixture.absenceDate, target.ID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			WITH make_up AS (
				INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
				VALUES ($1, $2, $3, $4)
				RETURNING id
			)
			INSERT INTO absence_sit_ins (absence_id, session_id) SELECT $5, id FROM make_up
		`, target.ID, adminID, fixture.startAt, fixture.endAt, id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	settings, err := q.AppSettingsGetWithPolicies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := q.AppSettingsUpdateAbsencePolicies(ctx, settings.AbsencePolicies); err != nil {
			t.Error(err)
		}
	})
	const template = `{"notifications":{"sms_special_approved_template":"{{absence_summary}}\n{{sit_in_summary}}"}}`
	if err := q.AppSettingsUpdateAbsencePolicies(ctx, []byte(template)); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Auth: staffFakeAuth{user: auth.AuthenticatedUser{ID: adminID, Role: "Admin"}},
		Q:    q, DB: pool, Log: slog.Default(), InstituteTZ: "Asia/Bangkok",
	})
	request := func(dryRun bool, wantStatus int) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{"ids": ids, "dry_run": dryRun})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/batch-send-success-sms", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", uuid.NewString())
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != wantStatus {
			t.Fatalf("dry_run=%v: status = %d, want %d: %s", dryRun, res.Code, wantStatus, res.Body)
		}
		return res
	}

	var preview struct {
		Preview struct {
			Phones  []string `json:"phones"`
			Message string   `json:"message"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(request(true, http.StatusOK).Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if got := preview.Preview.Phones; len(got) != 1 || got[0] != phone {
		t.Fatalf("preview recipients = %v, want [%s]", got, phone)
	}
	want := fmt.Sprintf("%s (28 Nov 2026); %s (29 Nov 2026)\nRank 3 Section 1 (2 Dec, 17:00 - 18:40); Rank 4 (1 Dec, 17:00 - 20:20)", mergeName, mergeName)
	if got := preview.Preview.Message; got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}

	var queued struct {
		NotificationID string `json:"notification_id"`
	}
	if err := json.Unmarshal(request(false, http.StatusAccepted).Body.Bytes(), &queued); err != nil {
		t.Fatal(err)
	}
	var message string
	if err := pool.QueryRow(ctx, "SELECT payload->>'sms_message' FROM notification_outbox WHERE id = $1", queued.NotificationID).Scan(&message); err != nil {
		t.Fatal(err)
	}
	if message != preview.Preview.Message {
		t.Fatalf("queued message = %q, want preview %q", message, preview.Preview.Message)
	}
}
