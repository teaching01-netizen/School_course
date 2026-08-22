package absenceshttp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
)

func TestSessionsInRangeQueryRestrictsToActiveCourse(t *testing.T) {
	sql := sessionsInRangeSelectSQL()

	for _, fragment := range []string{
		"LEFT JOIN subject_active_courses sac",
		"OR c.id = sac.course_id",
		"OR (ac.cycle_id IS NOT NULL AND c.cycle_id = ac.cycle_id)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sessions-in-range query should contain %q, SQL: %s", fragment, sql)
		}
	}
}

type activeCourseSeed struct {
	dbpool  *pgxpool.Pool
	wcode   string
	suffix  string
	subjID  uuid.UUID
	courses map[string]uuid.UUID // label -> course id
}

// seedActiveCourseFixture builds one subject with three courses: "old" and
// "current" in different cycles, "sibling" in the current cycle. The student is
// enrolled in all three and each course has one session inside the window.
func seedActiveCourseFixture(t *testing.T, dbpool *pgxpool.Pool) activeCourseSeed {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := uuid.NewString()[:8]
	seed := activeCourseSeed{
		dbpool:  dbpool,
		wcode:   "wactsess" + suffix,
		suffix:  suffix,
		courses: map[string]uuid.UUID{},
	}

	if _, err := dbpool.Exec(ctx, `
		INSERT INTO students (wcode, full_name) VALUES ($1, $2)
	`, seed.wcode, "Active Course Sess "+suffix); err != nil {
		t.Fatal(err)
	}
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO subjects (code, name) VALUES ($1, $2) RETURNING id
	`, "ASUBJ-"+suffix, "Active Sess Subject "+suffix).Scan(&seed.subjID); err != nil {
		t.Fatal(err)
	}

	var teacherID uuid.UUID
	if err := dbpool.QueryRow(ctx, `
		INSERT INTO users (username, role, password_hash)
		VALUES ($1, 'Teacher', 'x') RETURNING id
	`, "teacher-actsess-"+suffix).Scan(&teacherID); err != nil {
		t.Fatal(err)
	}

	cycles := map[string]string{"old": "CY-OLD-" + suffix, "current": "CY-CUR-" + suffix, "sibling": "CY-CUR-" + suffix}
	for _, cycleID := range cycles {
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO crm_cycles (id, label) VALUES ($1, $1) ON CONFLICT (id) DO NOTHING
		`, cycleID); err != nil {
			t.Fatal(err)
		}
	}
	for i, label := range []string{"old", "current", "sibling"} {
		var courseID uuid.UUID
		if err := dbpool.QueryRow(ctx, `
			INSERT INTO courses (code, name, subject_id, cycle_id)
			VALUES ($1, $2, $3, $4) RETURNING id
		`, "ACRS-"+label+"-"+suffix, "Active Sess "+label, seed.subjID, cycles[label]).Scan(&courseID); err != nil {
			t.Fatal(err)
		}
		seed.courses[label] = courseID
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO course_students (course_id, student_id, status)
			SELECT $1, id, 'enrolled' FROM students WHERE wcode = $2
		`, courseID, seed.wcode); err != nil {
			t.Fatal(err)
		}
		sessionStart := time.Now().UTC().AddDate(0, 0, 7+i)
		if _, err := dbpool.Exec(ctx, `
			INSERT INTO sessions (course_id, teacher_id, start_at, end_at)
			VALUES ($1, $2, $3, $4)
		`, courseID, teacherID, sessionStart, sessionStart.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	return seed
}

func (s activeCourseSeed) code(label string) string {
	return "ACRS-" + label + "-" + s.suffix
}

func setActiveCourseRow(t *testing.T, dbpool *pgxpool.Pool, subjID, courseID uuid.UUID) {
	t.Helper()
	if _, err := dbpool.Exec(context.Background(), `
		INSERT INTO subject_active_courses (subject_id, course_id)
		VALUES ($1, $2)
		ON CONFLICT (subject_id) DO UPDATE SET course_id = $2, updated_at = now()
	`, subjID, courseID); err != nil {
		t.Fatal(err)
	}
}

func clearActiveCourseRow(t *testing.T, dbpool *pgxpool.Pool, subjID uuid.UUID) {
	t.Helper()
	if _, err := dbpool.Exec(context.Background(), `
		DELETE FROM subject_active_courses WHERE subject_id = $1
	`, subjID); err != nil {
		t.Fatal(err)
	}
}

func querySessionCourseCodes(t *testing.T, dbpool *pgxpool.Pool, wcode string) map[string]bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := dbpool.Query(ctx, sessionsInRangeSelectSQL(), wcode, time.Now().UTC().AddDate(0, 0, -30), time.Now().UTC().AddDate(0, 0, 90))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	codes := map[string]bool{}
	for rows.Next() {
		var id, courseID, subjectID uuid.UUID
		var startAt, endAt time.Time
		var courseCode, courseName, subjectCode, subjectName, teacherName string
		if err := rows.Scan(&id, &startAt, &endAt, &courseID, &courseCode, &courseName, &subjectID, &subjectCode, &subjectName, &teacherName); err != nil {
			t.Fatal(err)
		}
		codes[courseCode] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return codes
}

func TestSessionsInRangeActiveCourseFiltering(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)

	seed := seedActiveCourseFixture(t, dbpool)

	t.Run("no_active_course_configured_shows_all_enrolled", func(t *testing.T) {
		clearActiveCourseRow(t, dbpool, seed.subjID)
		codes := querySessionCourseCodes(t, dbpool, seed.wcode)
		if len(codes) != 3 {
			t.Fatalf("expected sessions from all 3 enrolled courses, got %v", codes)
		}
	})

	t.Run("active_course_hides_other_cycles_keeps_same_cycle_siblings", func(t *testing.T) {
		setActiveCourseRow(t, dbpool, seed.subjID, seed.courses["current"])
		codes := querySessionCourseCodes(t, dbpool, seed.wcode)
		if codes[seed.code("old")] {
			t.Fatalf("old-cycle course should be hidden by active course, got %v", codes)
		}
		if !codes[seed.code("current")] || !codes[seed.code("sibling")] {
			t.Fatalf("active course and same-cycle sibling should remain, got %v", codes)
		}
	})

	t.Run("student_not_enrolled_in_active_course_keeps_all", func(t *testing.T) {
		// Point the active course at a course the student is NOT enrolled in.
		var outsider uuid.UUID
		outsiderCycle := "CY-OUT-" + seed.suffix
		if _, err := dbpool.Exec(context.Background(), `
			INSERT INTO crm_cycles (id, label) VALUES ($1, $1) ON CONFLICT (id) DO NOTHING
		`, outsiderCycle); err != nil {
			t.Fatal(err)
		}
		if err := dbpool.QueryRow(context.Background(), `
			INSERT INTO courses (code, name, subject_id, cycle_id)
			VALUES ($1, 'Outsider course', $2, $3) RETURNING id
		`, "ACRS-outsider-"+seed.suffix, seed.subjID, outsiderCycle).Scan(&outsider); err != nil {
			t.Fatal(err)
		}
		setActiveCourseRow(t, dbpool, seed.subjID, outsider)
		codes := querySessionCourseCodes(t, dbpool, seed.wcode)
		if len(codes) != 3 {
			t.Fatalf("student outside the active course should keep all enrolled courses, got %v", codes)
		}
	})
}

// The active-course restriction is a student-facing bookability concept. Staff
// reviewing a student's sessions must see every enrolled course, including
// stale cycles, so the staff SQL variant must not apply the filter.
func TestSessionsInRangeStaffQueryBypassesActiveCourse(t *testing.T) {
	sql := sessionsInRangeStaffSelectSQL()

	for _, fragment := range []string{"subject_active_courses", "sac."} {
		if strings.Contains(sql, fragment) {
			t.Fatalf("staff sessions-in-range query must not apply active-course filtering, but contains %q; SQL: %s", fragment, sql)
		}
	}
	if !strings.Contains(sql, "cs.status = 'enrolled'") {
		t.Fatalf("staff sessions-in-range query must still restrict to the student's enrolled courses; SQL: %s", sql)
	}
}

func TestSessionsInRangeStaffRequestShowsAllEnrolledCourses(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	migrateUpOncePending(t, databaseURL)
	dbpool := newPoolPending(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)

	seed := seedActiveCourseFixture(t, dbpool)
	setActiveCourseRow(t, dbpool, seed.subjID, seed.courses["current"])

	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Q:           q,
		DB:          dbpool,
		Log:         slog.Default(),
		InstituteTZ: "Asia/Bangkok",
		Auth:        staffFakeAuth{user: auth.AuthenticatedUser{ID: uuid.New(), Role: "Admin"}},
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	resp := staffDoRequest(t, server.URL, http.MethodGet,
		"/api/v1/absences/sessions-in-range?wcode="+seed.wcode+"&bypass_timing=true", nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Subjects []struct {
			CourseCode string `json:"course_code"`
		} `json:"subjects"`
	}
	staffParseResponse(t, resp, &out)

	codes := map[string]bool{}
	for _, subj := range out.Subjects {
		codes[subj.CourseCode] = true
	}
	for _, label := range []string{"old", "current", "sibling"} {
		if !codes[seed.code(label)] {
			t.Fatalf("staff sessions-in-range must include the %s course even when an active course is set, got %v", label, codes)
		}
	}
}
