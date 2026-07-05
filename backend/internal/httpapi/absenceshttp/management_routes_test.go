package absenceshttp

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sqldb "warwick-institute/internal/db"
)

var (
	migrationsOnceMgmt sync.Once
	migrationsErrMgmt  error
)

func requireTestDBMgmt(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	return url
}

func migrateUpOnceMgmt(t *testing.T, databaseURL string) {
	t.Helper()
	migrationsOnceMgmt.Do(func() {
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		} else {
			databaseURL = databaseURL + "?default_query_exec_mode=simple_protocol&statement_cache_capacity=0"
		}
		db, err := sql.Open("pgx", databaseURL)
		if err != nil {
			migrationsErrMgmt = err
			return
		}
		defer db.Close()
		_, _ = db.Exec(`DELETE FROM crm_rows`)
		if err := goose.SetDialect("postgres"); err != nil {
			migrationsErrMgmt = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			migrationsErrMgmt = context.Canceled
			return
		}
		migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "migrations"))
		migrationsErrMgmt = goose.UpContext(ctx, db, migrationsDir)
	})
	if migrationsErrMgmt != nil {
		t.Fatal(migrationsErrMgmt)
	}
}

func newPoolMgmt(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// seedAbsenceDeleteTestData inserts a student, subject, course, enrolment,
// a session, and a student_absence row. Returns the absence ID and student wcode.
func seedAbsenceDeleteTestData(t *testing.T, dbpool *pgxpool.Pool, q *sqldb.Queries, suffix string) (absenceID pgtype.UUID, wcode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wcode = "w" + suffix

	_, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`,
		wcode, "Test Student "+suffix)
	if err != nil {
		t.Fatal(err)
	}

	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: "SUBJ-" + suffix, Name: "Subject " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}

	var courseID pgtype.UUID
	err = dbpool.QueryRow(ctx,
		`INSERT INTO courses (code, name, subject_id) VALUES ($1, $2, $3) RETURNING id`,
		"COURSE-"+suffix, "Course "+suffix, subject.ID,
	).Scan(&courseID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dbpool.Exec(ctx,
		`INSERT INTO course_students (course_id, student_id, status)
		 SELECT $1, s.id, 'enrolled' FROM students s WHERE s.wcode = $2`,
		courseID, wcode)
	if err != nil {
		t.Fatal(err)
	}

	var teacherID pgtype.UUID
	err = dbpool.QueryRow(ctx,
		`INSERT INTO users (username, role, password_hash)
		 VALUES ($1, $2, $3) RETURNING id`,
		"teacher-"+suffix, "Teacher", "x").Scan(&teacherID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = dbpool.Exec(ctx,
		`INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
		 VALUES ($1, $2, now(), now() + interval '1 hour')`,
		courseID, teacherID)
	if err != nil {
		t.Fatal(err)
	}

	err = dbpool.QueryRow(ctx, `
		INSERT INTO student_absences (wcode, course_id, date_from, date_to, status, reason)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, wcode, courseID, "2026-06-01", "2026-06-01", "cancelled", "Test absence").Scan(&absenceID)
	if err != nil {
		t.Fatal(err)
	}

	return absenceID, wcode
}

func TestAbsenceHardDelete_CancelledAbsenceDeletedWithStaleVersion(t *testing.T) {
	databaseURL := requireTestDBMgmt(t)
	migrateUpOnceMgmt(t, databaseURL)
	dbpool := newPoolMgmt(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	absenceID, _ := seedAbsenceDeleteTestData(t, dbpool, q, suffix)

	// Fetch the current version from the DB
	ctx := context.Background()
	var dbVersion int32
	err := dbpool.QueryRow(ctx, `SELECT version FROM student_absences WHERE id = $1`, absenceID).Scan(&dbVersion)
	if err != nil {
		t.Fatal(err)
	}

	// Attempt to hard-delete with a stale version (dbVersion + 1) — should succeed
	// because the absence is already cancelled (our fix allows this).
	rows, err := q.AbsenceHardDelete(ctx, absenceID, dbVersion+999)
	if err != nil {
		t.Fatalf("AbsenceHardDelete with stale version on cancelled absence should succeed, got: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row deleted, got %d", rows)
	}

	// Verify the row is actually gone
	var count int32
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*)::int4 FROM student_absences WHERE id = $1`, absenceID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("absence row was not actually deleted")
	}
}

func TestAbsenceHardDelete_NonCancelledAbsenceRejectsStaleVersion(t *testing.T) {
	databaseURL := requireTestDBMgmt(t)
	migrateUpOnceMgmt(t, databaseURL)
	dbpool := newPoolMgmt(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	// Seed with a non-cancelled absence (pending status)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wcode := "w" + suffix
	_, err := dbpool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, $2)`, wcode, "Student "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{Code: "SUBJ-" + suffix, Name: "Subject"})
	if err != nil {
		t.Fatal(err)
	}
	var courseID pgtype.UUID
	err = dbpool.QueryRow(ctx, `INSERT INTO courses (code, name, subject_id) VALUES ($1,$2,$3) RETURNING id`,
		"CRS-"+suffix, "Course", subject.ID).Scan(&courseID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbpool.Exec(ctx, `INSERT INTO course_students (course_id, student_id, status) SELECT $1, s.id, 'enrolled' FROM students s WHERE s.wcode=$2`,
		courseID, wcode)
	if err != nil {
		t.Fatal(err)
	}
	var absenceID pgtype.UUID
	err = dbpool.QueryRow(ctx, `
		INSERT INTO student_absences (wcode, course_id, date_from, date_to, status, reason)
		VALUES ($1,$2,$3,$4,'pending','Test') RETURNING id
	`, wcode, courseID, "2026-06-01", "2026-06-01").Scan(&absenceID)
	if err != nil {
		t.Fatal(err)
	}

	var dbVersion int32
	err = dbpool.QueryRow(ctx, `SELECT version FROM student_absences WHERE id = $1`, absenceID).Scan(&dbVersion)
	if err != nil {
		t.Fatal(err)
	}

	// Attempting to hard-delete with a stale version should fail (pgx.ErrNoRows)
	_, err = q.AbsenceHardDelete(ctx, absenceID, dbVersion+999)
	if !sqldb.IsNoRows(err) {
		t.Fatalf("AbsenceHardDelete with stale version on pending absence should return ErrNoRows, got: %v", err)
	}

	// Verify the row still exists
	var count int32
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*)::int4 FROM student_absences WHERE id = $1`, absenceID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("absence row should still exist after rejected delete")
	}
}

func TestAbsenceHardDelete_DeletedWithCorrectVersion(t *testing.T) {
	databaseURL := requireTestDBMgmt(t)
	migrateUpOnceMgmt(t, databaseURL)
	dbpool := newPoolMgmt(t, databaseURL)
	t.Cleanup(dbpool.Close)

	q := sqldb.New(dbpool)
	suffix := uuid.New().String()[:8]

	absenceID, _ := seedAbsenceDeleteTestData(t, dbpool, q, suffix)

	ctx := context.Background()
	var dbVersion int32
	err := dbpool.QueryRow(ctx, `SELECT version FROM student_absences WHERE id = $1`, absenceID).Scan(&dbVersion)
	if err != nil {
		t.Fatal(err)
	}

	// Delete with the correct version — should succeed
	rows, err := q.AbsenceHardDelete(ctx, absenceID, dbVersion)
	if err != nil {
		t.Fatalf("AbsenceHardDelete with correct version should succeed, got: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 row deleted, got %d", rows)
	}

	var count int32
	err = dbpool.QueryRow(ctx, `SELECT COUNT(*)::int4 FROM student_absences WHERE id = $1`, absenceID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("absence row was not actually deleted")
	}
}

func TestParseAbsenceSettings_LegacyJSON_MissingSpecialTemplate(t *testing.T) {
	legacyJSON := []byte(`{"notifications":{"sms_parent_enabled":true,"sms_parent_template":"OTP {{code}}","sms_success_template":"Normal {{absence_summary}}","allow_submit_without_otp":false}}`)
	settings := parseAbsenceSettings(legacyJSON)

	if settings.Notifications.SmsSuccessTemplate != "Normal {{absence_summary}}" {
		t.Errorf("SmsSuccessTemplate = %q, want %q", settings.Notifications.SmsSuccessTemplate, "Normal {{absence_summary}}")
	}
	defaults := defaultAbsenceSettings()
	if settings.Notifications.SmsSpecialApprovedTemplate != defaults.Notifications.SmsSpecialApprovedTemplate {
		t.Errorf("SmsSpecialApprovedTemplate should be filled from defaults, got %q", settings.Notifications.SmsSpecialApprovedTemplate)
	}
}

func TestParseAbsenceSettings_NewJSON_WithSpecialTemplate(t *testing.T) {
	raw := []byte(`{"notifications":{"sms_special_approved_template":"Custom special template"}}`)
	settings := parseAbsenceSettings(raw)

	if settings.Notifications.SmsSpecialApprovedTemplate != "Custom special template" {
		t.Errorf("SmsSpecialApprovedTemplate = %q, want %q", settings.Notifications.SmsSpecialApprovedTemplate, "Custom special template")
	}
}

func TestParseAbsenceSettings_EmptyJSON(t *testing.T) {
	settings := parseAbsenceSettings([]byte(`{}`))
	defaults := defaultAbsenceSettings()

	if settings.Notifications.SmsParentEnabled != defaults.Notifications.SmsParentEnabled {
		t.Errorf("SmsParentEnabled = %v, want %v", settings.Notifications.SmsParentEnabled, defaults.Notifications.SmsParentEnabled)
	}
	if settings.Notifications.SmsSpecialApprovedTemplate != defaults.Notifications.SmsSpecialApprovedTemplate {
		t.Errorf("SmsSpecialApprovedTemplate should default, got %q", settings.Notifications.SmsSpecialApprovedTemplate)
	}
}

func TestParseAbsenceSettings_PartialJSON(t *testing.T) {
	raw := []byte(`{"notifications":{"sms_success_template":"Custom normal"}}`)
	settings := parseAbsenceSettings(raw)

	if settings.Notifications.SmsSuccessTemplate != "Custom normal" {
		t.Errorf("SmsSuccessTemplate = %q, want %q", settings.Notifications.SmsSuccessTemplate, "Custom normal")
	}
	defaults := defaultAbsenceSettings()
	if settings.Notifications.SmsSpecialApprovedTemplate != defaults.Notifications.SmsSpecialApprovedTemplate {
		t.Errorf("SmsSpecialApprovedTemplate should be default, got %q", settings.Notifications.SmsSpecialApprovedTemplate)
	}
}

func TestValidateAbsenceSettings_SpecialTemplateLength(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{"empty is valid", "", false},
		{"500 chars is valid", string(make([]rune, 500)), false},
		{"501 chars is invalid", string(make([]rune, 501)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := defaultAbsenceSettings()
			settings.Notifications.SmsSpecialApprovedTemplate = tt.template
			err := validateAbsenceSettings(settings)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAbsenceSettings() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidAbsenceStatus_SpecialApproved(t *testing.T) {
	if !validAbsenceStatus("special_approved") {
		t.Fatal("special_approved should be a valid status")
	}
}

func TestValidTransition_SpecialApproved(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want bool
	}{
		{"pending to special_approved", "pending", "special_approved", true},
		{"reviewed to special_approved", "reviewed", "special_approved", true},
		{"actioned to special_approved", "actioned", "special_approved", true},
		{"cancelled to special_approved", "cancelled", "special_approved", false},
		{"special_approved to pending", "special_approved", "pending", false},
		{"special_approved to reviewed", "special_approved", "reviewed", false},
		{"special_approved to actioned", "special_approved", "actioned", false},
		{"special_approved to cancelled", "special_approved", "cancelled", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("validTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestStatusAuditAction_SpecialApproved(t *testing.T) {
	tests := []struct {
		current, next, want string
	}{
		{"pending", "special_approved", "special_approved"},
		{"reviewed", "special_approved", "special_approved"},
		{"actioned", "special_approved", "special_approved"},
	}
	for _, tt := range tests {
		t.Run(tt.current+"_to_"+tt.next, func(t *testing.T) {
			got := statusAuditAction(tt.current, tt.next)
			if got != tt.want {
				t.Errorf("statusAuditAction(%q, %q) = %q, want %q", tt.current, tt.next, got, tt.want)
			}
		})
	}
}

// --- Section B: Email config tests ---

func TestDefaultAbsenceSettings_EmailDefaults(t *testing.T) {
	settings := defaultAbsenceSettings()
	if settings.Notifications.EmailSuccessEnabled {
		t.Error("default should have email disabled")
	}
	if settings.Notifications.EmailSuccessSubject == "" {
		t.Error("default email subject should not be empty")
	}
	if settings.Notifications.EmailSuccessBody == "" {
		t.Error("default email body should not be empty")
	}
}

func TestEmailSuccessConfig_Enabled(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessEnabled = true
	cfg := settings.emailSuccessConfig()
	if !cfg.Enabled {
		t.Error("expected Enabled to be true")
	}
}

func TestEmailSuccessConfig_Disabled(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessEnabled = false
	cfg := settings.emailSuccessConfig()
	if cfg.Enabled {
		t.Error("expected Enabled to be false")
	}
}

func TestEmailSuccessConfig_CustomSubject(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessSubject = "Hi {{student_name}}"
	cfg := settings.emailSuccessConfig()
	if cfg.Subject != "Hi {{student_name}}" {
		t.Errorf("Subject = %q, want %q", cfg.Subject, "Hi {{student_name}}")
	}
}

func TestEmailSuccessConfig_CustomBody(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessBody = "<p>Hello</p>"
	cfg := settings.emailSuccessConfig()
	if cfg.Body != "<p>Hello</p>" {
		t.Errorf("Body = %q, want %q", cfg.Body, "<p>Hello</p>")
	}
}

func TestEmailSuccessConfig_EmptySubjectUsesDefault(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessSubject = ""
	cfg := settings.emailSuccessConfig()
	defaultCfg := defaultEmailSuccessConfig()
	if cfg.Subject != defaultCfg.Subject {
		t.Errorf("empty subject should use default, got %q", cfg.Subject)
	}
}

func TestEmailSuccessConfig_EmptyBodyUsesDefault(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessBody = ""
	cfg := settings.emailSuccessConfig()
	defaultCfg := defaultEmailSuccessConfig()
	if cfg.Body != defaultCfg.Body {
		t.Errorf("empty body should use default, got %q", cfg.Body)
	}
}

func TestParseAbsenceSettings_EmailFieldsParsed(t *testing.T) {
	raw := []byte(`{"notifications":{"email_success_enabled":true,"email_success_subject":"Custom Subject","email_success_body":"<p>Custom</p>"}}`)
	settings := parseAbsenceSettings(raw)
	if !settings.Notifications.EmailSuccessEnabled {
		t.Error("expected email_success_enabled to be true")
	}
	if settings.Notifications.EmailSuccessSubject != "Custom Subject" {
		t.Errorf("email_success_subject = %q, want %q", settings.Notifications.EmailSuccessSubject, "Custom Subject")
	}
	if settings.Notifications.EmailSuccessBody != "<p>Custom</p>" {
		t.Errorf("email_success_body = %q, want %q", settings.Notifications.EmailSuccessBody, "<p>Custom</p>")
	}
}

func TestParseAbsenceSettings_LegacyMissingEmailFields(t *testing.T) {
	raw := []byte(`{"notifications":{"sms_parent_enabled":true,"sms_success_template":"Hi"}}`)
	settings := parseAbsenceSettings(raw)
	if settings.Notifications.EmailSuccessEnabled {
		t.Error("legacy JSON should default email to disabled")
	}
	if settings.Notifications.EmailSuccessSubject != "" {
		t.Errorf("legacy JSON without email subject should be empty, got %q", settings.Notifications.EmailSuccessSubject)
	}
	if settings.Notifications.EmailSuccessBody != "" {
		t.Errorf("legacy JSON without email body should be empty, got %q", settings.Notifications.EmailSuccessBody)
	}
	// The emailSuccessConfig() method applies defaults for empty fields
	cfg := settings.emailSuccessConfig()
	defaults := defaultEmailSuccessConfig()
	if cfg.Subject != defaults.Subject {
		t.Errorf("emailSuccessConfig should fall back to default subject, got %q", cfg.Subject)
	}
	if cfg.Body != defaults.Body {
		t.Errorf("emailSuccessConfig should fall back to default body, got %q", cfg.Body)
	}
}

func TestParseAbsenceSettings_EmailDisabled(t *testing.T) {
	raw := []byte(`{"notifications":{"email_success_enabled":false}}`)
	settings := parseAbsenceSettings(raw)
	if settings.Notifications.EmailSuccessEnabled {
		t.Error("expected email_success_enabled to be false")
	}
}

func TestValidateAbsenceSettings_EmailSubjectTooLong(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessSubject = string(make([]rune, 201))
	err := validateAbsenceSettings(settings)
	if err == nil {
		t.Error("expected error for email_success_subject > 200 chars")
	}
	if !strings.Contains(err.Error(), "email_success_subject") {
		t.Errorf("error should mention email_success_subject, got %q", err.Error())
	}
}

func TestValidateAbsenceSettings_EmailBodyTooLong(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessBody = string(make([]rune, 15001))
	err := validateAbsenceSettings(settings)
	if err == nil {
		t.Error("expected error for email_success_body > 15000 chars")
	}
	if !strings.Contains(err.Error(), "email_success_body") {
		t.Errorf("error should mention email_success_body, got %q", err.Error())
	}
}

func TestValidateAbsenceSettings_EmailSubjectAtLimit(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessSubject = string(make([]rune, 200))
	err := validateAbsenceSettings(settings)
	if err != nil {
		t.Errorf("200 chars should be valid, got error: %v", err)
	}
}

func TestValidateAbsenceSettings_EmailBodyAtLimit(t *testing.T) {
	settings := defaultAbsenceSettings()
	settings.Notifications.EmailSuccessBody = string(make([]rune, 15000))
	err := validateAbsenceSettings(settings)
	if err != nil {
		t.Errorf("15000 chars should be valid, got error: %v", err)
	}
}
