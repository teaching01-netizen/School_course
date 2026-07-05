package absenceshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type absenceLimitFakeAuth struct {
	user auth.AuthenticatedUser
	err  error
}

func (f absenceLimitFakeAuth) RequireUser(_ context.Context, _ *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}

func (absenceLimitFakeAuth) HandleLogin(_ http.ResponseWriter, _ *http.Request) error  { return nil }
func (absenceLimitFakeAuth) HandleLogout(_ http.ResponseWriter, _ *http.Request) error { return nil }

func requireAbsenceLimitTestDB(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func seedAbsenceLimitTestData(t *testing.T, q *sqldb.Queries, dbpool *pgxpool.Pool, prefix string, totalSessions int) (studentWcode, subjectID, courseID string, sessionIDs []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")

	subj, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: prefix + "-SUBJ-" + suffix,
		Name: prefix + " Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	subjectIDStr, _ := uuidString(subj.ID)
	subjectID = subjectIDStr

	course, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: prefix + "-CRS-" + suffix,
		Name: prefix + " Course " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	courseIDStr, _ := uuidString(course.ID)
	courseID = courseIDStr
	if _, err := dbpool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", subj.ID, course.ID); err != nil {
		t.Fatal(err)
	}

	studentWcode = "w" + strings.ToLower(prefix) + "-" + suffix
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{
		Wcode:    studentWcode,
		FullName: prefix + " Student " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{
		CourseID:  course.ID,
		StudentID: student.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// Create sessions
	sessionIDs = make([]string, 0, totalSessions)
	for i := 0; i < totalSessions; i++ {
		startDate := time.Date(2026, 6, i+1, 9, 0, 0, 0, time.UTC)
		endDate := startDate.Add(90 * time.Minute)
		sess, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID: course.ID,
			StartAt:  pgtype.Timestamptz{Time: startDate, Valid: true},
			EndAt:    pgtype.Timestamptz{Time: endDate, Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		sessionIDStr, _ := uuidString(sess.ID)
		sessionIDs = append(sessionIDs, sessionIDStr)
	}

	return studentWcode, subjectID, courseID, sessionIDs
}

func TestAbsenceLimit_SingleCreate_403WhenLimitExceeded(t *testing.T) {
	databaseURL := requireAbsenceLimitTestDB(t)

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	dbpool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer dbpool.Close()

	q := sqldb.New(dbpool)
	wcode, subjectIDStr, courseIDStr, _ := seedAbsenceLimitTestData(t, q, dbpool, "LMT", 10)

	// Parse subject and course IDs
	var subjectID pgtype.UUID
	if err := subjectID.Scan(subjectIDStr); err != nil {
		t.Fatal(err)
	}
	var courseID pgtype.UUID
	if err := courseID.Scan(courseIDStr); err != nil {
		t.Fatal(err)
	}

	// Create 2 existing absence records (20% limit reached)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
			Wcode:    wcode,
			CourseID: courseID,
			DateFrom: pgtype.Date{Time: time.Date(2026, 6, i+1, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:   pgtype.Date{Time: time.Date(2026, 6, i+1, 0, 0, 0, 0, time.UTC), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		// Link to subject
		if err := q.AbsenceSetSubmissionMetadata(ctx, absence.ID, subjectID, pgtype.Text{}, "Test Student", pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.UUID{}); err != nil {
			t.Fatal(err)
		}
	}

	s := &server{
		deps: httpdeps.Deps{
			Q:           q,
			DB:          dbpool,
			Log:         slog.Default(),
			InstituteTZ: "Asia/Bangkok",
			Auth:        absenceLimitFakeAuth{user: auth.AuthenticatedUser{Role: "Student"}},
		},
		a: httpadapter.Adapter{},
	}

	body := map[string]any{
		"wcode":              wcode,
		"subject_id":         subjectIDStr,
		"course_id":          courseIDStr,
		"date_from":          "2026-06-15",
		"date_to":            "2026-06-15",
		"missed_session_ids": []string{},
	}
	reqBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	s.handleAbsenceCreate(w, req)

	// Should return 403 because limit is exceeded
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error_code"] != "absence_limit_exceeded" {
		t.Fatalf("expected error_code 'absence_limit_exceeded', got %v", resp["error_code"])
	}
}

func TestAbsenceLimit_SessionsInRange_AbsenceRateExceededFlag(t *testing.T) {
	databaseURL := requireAbsenceLimitTestDB(t)

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	dbpool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer dbpool.Close()

	q := sqldb.New(dbpool)
	wcode, subjectIDStr, courseIDStr, _ := seedAbsenceLimitTestData(t, q, dbpool, "RTE", 10)

	// Parse subject ID
	var subjectID pgtype.UUID
	if err := subjectID.Scan(subjectIDStr); err != nil {
		t.Fatal(err)
	}

	// Create 2 existing absence records (20% limit reached)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
			Wcode:    wcode,
			CourseID: pgtype.UUID{},
			DateFrom: pgtype.Date{Time: time.Date(2026, 6, i+1, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:   pgtype.Date{Time: time.Date(2026, 6, i+1, 0, 0, 0, 0, time.UTC), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := q.AbsenceSetSubmissionMetadata(ctx, absence.ID, subjectID, pgtype.Text{}, "Test Student", pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.UUID{}); err != nil {
			t.Fatal(err)
		}
	}

	s := &server{
		deps: httpdeps.Deps{
			Q:           q,
			DB:          dbpool,
			Log:         slog.Default(),
			InstituteTZ: "Asia/Bangkok",
			Auth:        absenceLimitFakeAuth{user: auth.AuthenticatedUser{Role: "Student"}},
		},
		a: httpadapter.Adapter{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/absences/sessions-in-range?subject_ids="+subjectIDStr+"&date_from=2026-06-01&date_to=2026-06-30&wcode="+wcode, nil)
	w := httptest.NewRecorder()

	s.handleSessionsInRange(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Subjects []struct {
			CourseID             string `json:"course_id"`
			AbsenceRateExceeded  bool   `json:"absence_rate_exceeded"`
			ExistingAbsenceCount int32  `json:"existing_absence_count"`
			TotalSessionCount    int32  `json:"total_session_count"`
		} `json:"subjects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if len(resp.Subjects) == 0 {
		t.Fatal("expected at least one subject in response")
	}

	subject := resp.Subjects[0]
	if subject.CourseID != courseIDStr {
		t.Fatalf("expected course_id %s, got %s", courseIDStr, subject.CourseID)
	}
	if !subject.AbsenceRateExceeded {
		t.Fatal("expected absence_rate_exceeded to be true")
	}
	if subject.ExistingAbsenceCount != 2 {
		t.Fatalf("expected existing_absence_count 2, got %d", subject.ExistingAbsenceCount)
	}
	if subject.TotalSessionCount != 10 {
		t.Fatalf("expected total_session_count 10, got %d", subject.TotalSessionCount)
	}
}

func TestAbsenceLimit_BatchCreate_403WhenLimitExceeded(t *testing.T) {
	databaseURL := requireAbsenceLimitTestDB(t)

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	dbpool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer dbpool.Close()

	q := sqldb.New(dbpool)
	wcode, subjectIDStr, courseIDStr, _ := seedAbsenceLimitTestData(t, q, dbpool, "BAT", 10)

	// Parse subject and course IDs
	var subjectID pgtype.UUID
	if err := subjectID.Scan(subjectIDStr); err != nil {
		t.Fatal(err)
	}
	var courseID pgtype.UUID
	if err := courseID.Scan(courseIDStr); err != nil {
		t.Fatal(err)
	}

	// Create 2 existing absence records (20% limit reached)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		absence, err := q.AbsenceCreate(ctx, sqldb.AbsenceCreateParams{
			Wcode:    wcode,
			CourseID: courseID,
			DateFrom: pgtype.Date{Time: time.Date(2026, 6, i+1, 0, 0, 0, 0, time.UTC), Valid: true},
			DateTo:   pgtype.Date{Time: time.Date(2026, 6, i+1, 0, 0, 0, 0, time.UTC), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := q.AbsenceSetSubmissionMetadata(ctx, absence.ID, subjectID, pgtype.Text{}, "Test Student", pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.UUID{}); err != nil {
			t.Fatal(err)
		}
	}

	s := &server{
		deps: httpdeps.Deps{
			Q:           q,
			DB:          dbpool,
			Log:         slog.Default(),
			InstituteTZ: "Asia/Bangkok",
			Auth:        absenceLimitFakeAuth{user: auth.AuthenticatedUser{Role: "Student"}},
		},
		a: httpadapter.Adapter{},
	}

	body := map[string]any{
		"wcode": wcode,
		"items": []map[string]any{
			{
				"subject_id":        subjectIDStr,
				"course_id":         courseIDStr,
				"date_from":         "2026-06-15",
				"date_to":           "2026-06-15",
				"missed_session_ids": []string{},
			},
		},
	}
	reqBody, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/batch", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()

	s.handleAbsenceBatchCreate(w, req)

	// Should return 403 because limit is exceeded
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["error_code"] != "absence_limit_exceeded" {
		t.Fatalf("expected error_code 'absence_limit_exceeded', got %v", resp["error_code"])
	}
}
