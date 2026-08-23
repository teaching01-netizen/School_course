package absenceshttp

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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
