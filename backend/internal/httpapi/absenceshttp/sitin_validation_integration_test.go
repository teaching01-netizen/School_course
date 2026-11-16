package absenceshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/absences/sitinresolver"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

// Observe real database work rather than replace policy or attendance logic.
type submitValidationDB struct {
	sqldb.DBTX
	conflictIDs  [][]pgtype.UUID
	students     int
	conflictErr  error
	sessionLoads map[pgtype.UUID]int
}

func (d *submitValidationDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if strings.Contains(query, "name: StudentGetByWCode") {
		d.students++
	}
	return d.DBTX.QueryRow(ctx, query, args...)
}

func (d *submitValidationDB) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	if strings.Contains(query, "WHERE course_id IN (") && strings.Contains(query, "SELECT id, course_id, room_id, start_at, end_at") && d.sessionLoads != nil {
		d.sessionLoads[args[0].(pgtype.UUID)]++
	}
	if strings.Contains(query, "WITH overlapping AS MATERIALIZED") {
		d.conflictIDs = append(d.conflictIDs, append([]pgtype.UUID(nil), args[1].([]pgtype.UUID)...))
		if d.conflictErr != nil {
			return nil, d.conflictErr
		}
	}
	return d.DBTX.Query(ctx, query, args...)
}

func TestSitInSubmitValidationResolvesOnceAndChecksOnlySelectedSessions(t *testing.T) {
	c := seedSatCycle(t)
	c.applyAll(t)
	wcode, _ := c.student(t, "rb2")
	ctx := context.Background()
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	db := &submitValidationDB{DBTX: tx}
	q := sqldb.New(db)
	s := &server{deps: httpdeps.Deps{Q: q, InstituteTZ: "Asia/Bangkok", SitInResolver: sitinresolver.New(q, "Asia/Bangkok")}}
	for _, tc := range []struct {
		target  string
		indices []int
	}{
		{"rb1", []int{1}},
		{"rr5", []int{1}},
		{"rr4", []int{1}},
		{"rr5", []int{0, 1, 2}},
	} {
		t.Run(fmt.Sprintf("%s/%d_sessions", tc.target, len(tc.indices)), func(t *testing.T) {
			target := tc.target
			var absenceID pgtype.UUID
			date := c.courses["rb2"].Starts[1].In(c.loc).Format("2006-01-02")
			if err := tx.QueryRow(ctx, `INSERT INTO student_absences(wcode, course_id, subject_id, sit_in_course_id, date_from, date_to)
				VALUES ($1,$2,$3,$4,$5,$5) RETURNING id`, wcode, c.courses["rb2"].ID, c.subjectID, c.courses[target].ID, date).Scan(&absenceID); err != nil {
				t.Fatal(err)
			}
			selected := make([]pgtype.UUID, 0, len(tc.indices))
			for _, index := range tc.indices {
				selected = append(selected, makeUUID(c.courses[target].Sessions[index]))
			}
			db.students, db.conflictIDs = 0, nil
			db.sessionLoads = make(map[pgtype.UUID]int)
			if err := s.validateSitInCandidates(ctx, q, absenceID, selected); err != nil {
				t.Fatalf("valid selection at %s rejected: %v", target, err)
			}
			if db.students != 1 {
				t.Errorf("student resolutions = %d, want 1", db.students)
			}
			if len(db.conflictIDs) != 1 || !slices.Equal(db.conflictIDs[0], selected) {
				t.Errorf("expected-class checks = %v, want just selected sessions %v once", db.conflictIDs, selected)
			}
			if len(db.sessionLoads) != 4 {
				t.Fatalf("loaded sessions for %d courses, want source and three targets", len(db.sessionLoads))
			}
			for courseID, loads := range db.sessionLoads {
				if loads > 1 {
					t.Errorf("course %s sessions loaded %d times, want at most once", courseID, loads)
				}
			}
		})
	}
}

func TestSitInSubmitValidationKeepsExpectedClassAndPolicyChecks(t *testing.T) {
	f := seedPolicyAudit(t)
	ctx := context.Background()
	absenceID := f.createAuditAbsence(t)
	if _, err := f.tx.Exec(ctx, `UPDATE student_absences SET subject_id=$2, sit_in_course_id=$3 WHERE id=$1`, absenceID, f.subject, f.target); err != nil {
		t.Fatal(err)
	}
	s := &server{deps: httpdeps.Deps{Q: f.q, InstituteTZ: "Asia/Bangkok"}}
	for _, tc := range []struct {
		name    string
		session pgtype.UUID
		valid   bool
	}{
		{"adjacent", f.adjacent, true},
		{"overlaps expected class", f.overlapping, false},
		{"outside policy", f.unrelated, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := s.validateSitInCandidates(ctx, f.q, absenceID, []pgtype.UUID{tc.session})
			if tc.valid && err != nil || !tc.valid && !errors.Is(err, errInvalidSitInSelection) {
				t.Fatalf("validation error = %v, want valid=%t", err, tc.valid)
			}
		})
	}
}

func TestSitInSubmitValidationErrorsAreSafe(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"deadline", fmt.Errorf("private query details: %w", context.DeadlineExceeded), http.StatusGatewayTimeout, "timeout"},
		{"database", fmt.Errorf("private query details"), http.StatusInternalServerError, "internal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := seedFinalSessionFixture(t)
			ctx := context.Background()
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			db := &submitValidationDB{DBTX: tx, conflictErr: tc.err}
			q := sqldb.New(db)
			s := &server{deps: httpdeps.Deps{Q: q, InstituteTZ: "Asia/Bangkok"}, a: httpadapter.New(nil, nil)}
			settings := defaultAbsenceSettings()
			settings.Form.AbsenceLimitPercent = 100
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/test", nil)
			method, target := "physical", f.targetCourseID
			_, ok := s.createAbsenceRecordTx(w, r, q, tx, settings, f.wcode, pgtype.Text{}, pgtype.Text{String: "Medical appointment", Valid: true}, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, batchAbsenceCreateItem{
				SubjectID: f.subjectID, CourseID: f.courseID, DateFrom: f.date, DateTo: f.date,
				SitInMethod: &method, SitInCourseID: &target, SitInSessionIDs: []string{f.earlierID}, MissedSessionIDs: []string{f.missedID},
			})
			if ok || w.Code != tc.status {
				t.Fatalf("ok=%t status=%d, want %d; body=%s", ok, w.Code, tc.status, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) || strings.Contains(w.Body.String(), "private query details") || !strings.Contains(strings.ToLower(w.Body.String()), "try again") {
				t.Fatalf("unsafe or unhelpful response: %s", w.Body.String())
			}
		})
	}
}
