package absenceshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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
	calls        int
}

func (d *submitValidationDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	d.calls++
	if strings.Contains(query, "name: StudentGetByWCode") {
		d.students++
	}
	return d.DBTX.QueryRow(ctx, query, args...)
}

func (d *submitValidationDB) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	d.calls++
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
			if err := s.validateSitInCandidates(ctx, q, absenceID, selected, true); err != nil {
				t.Fatalf("valid selection at %s rejected: %v", target, err)
			}
			if db.students != 1 {
				t.Errorf("student resolutions = %d, want 1", db.students)
			}
			if len(db.conflictIDs) != 1 || !slices.Equal(db.conflictIDs[0], selected) {
				t.Errorf("expected-class checks = %v, want just selected sessions %v once", db.conflictIDs, selected)
			}
			if len(db.sessionLoads) != 2 {
				t.Fatalf("loaded sessions for %d courses, want only source and selected target", len(db.sessionLoads))
			}
			for courseID, loads := range db.sessionLoads {
				if loads > 1 {
					t.Errorf("course %s sessions loaded %d times, want at most once", courseID, loads)
				}
			}
		})
	}
}

func TestSitInSubmitValidationQueriesDoNotGrowWithUnselectedMappings(t *testing.T) {
	c := seedSatCycle(t)
	c.applyAll(t)
	wcode, _ := c.student(t, "rb2")
	ctx := context.Background()
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var absenceID pgtype.UUID
	date := c.courses["rb2"].Starts[1].In(c.loc).Format("2006-01-02")
	if err := tx.QueryRow(ctx, `INSERT INTO student_absences(wcode, course_id, subject_id, sit_in_course_id, date_from, date_to)
		VALUES ($1,$2,$3,$4,$5,$5) RETURNING id`, wcode, c.courses["rb2"].ID, c.subjectID, c.courses["rr5"].ID, date).Scan(&absenceID); err != nil {
		t.Fatal(err)
	}
	db := &submitValidationDB{DBTX: tx}
	q := sqldb.New(db)
	s := &server{deps: httpdeps.Deps{Q: q, InstituteTZ: "Asia/Bangkok"}}
	measure := func(name string) int {
		t.Helper()
		var elapsed []time.Duration
		var calls int
		for i := 0; i < 21; i++ {
			db.calls = 0
			start := time.Now()
			if err := s.validateSitInCandidates(ctx, q, absenceID, []pgtype.UUID{makeUUID(c.courses["rr5"].Sessions[1])}, true); err != nil {
				t.Fatal(err)
			}
			if i > 0 { // Warm the connection and query plans before sampling.
				elapsed = append(elapsed, time.Since(start))
			}
			calls = db.calls
		}
		slices.Sort(elapsed)
		t.Logf("%s: queries=%d p50=%s p95=%s (20 local samples)", name, calls, elapsed[10], elapsed[18])
		return calls
	}
	before := measure("standalone mappings")
	for i, key := range []string{"r3s1", "r3s2", "r3s3", "r2", "r1", "rm"} {
		var group, peer pgtype.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO course_merge_groups(name,level) VALUES($1,4) RETURNING id`, fmt.Sprintf("perf-%s-%d", c.suffix, i)).Scan(&group); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO courses(code,name,subject_id,student_booking_enabled)
			VALUES($1,$1,$2,true) RETURNING id`, fmt.Sprintf("perf-peer-%s-%d", c.suffix, i), c.subjectID).Scan(&peer); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO course_merge_group_members(group_id,course_id,position) VALUES($1,$2,1),($1,$3,2)`, group, c.courses[key].ID, peer); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE sat_verbal_policy_mappings SET course_id=NULL,merge_group_id=$2 WHERE course_id=$1`, c.courses[key].ID, group); err != nil {
			t.Fatal(err)
		}
	}
	if after := measure("six unrelated merged mappings"); after != before {
		t.Errorf("validation database calls grew from %d to %d for unrelated merged mappings", before, after)
	}
	// Compare batched metadata with the original single-course readers, including
	// group-level overrides and member ordering, before using it in policy checks.
	mappings, err := q.SatVerbalPolicyMappingsList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var expected []sqldb.SatVerbalMappedCourse
	for _, mapping := range mappings {
		if !mapping.MergeGroupID.Valid {
			expected = append(expected, sqldb.SatVerbalMappedCourse{Course: satVerbalCourseFromMapping(mapping), PolicyRule: mapping.PolicyRule})
			continue
		}
		group, err := q.CourseMergeGroupGet(ctx, mapping.MergeGroupID)
		if err != nil {
			t.Fatal(err)
		}
		members, err := q.CourseMergeGroupCourseIDs(ctx, mapping.MergeGroupID)
		if err != nil {
			t.Fatal(err)
		}
		for _, member := range members {
			course, err := q.CourseSubjectByID(ctx, member)
			if err != nil {
				t.Fatal(err)
			}
			expected = append(expected, sqldb.SatVerbalMappedCourse{Course: course, PolicyRule: mapping.PolicyRule, MergeGroupName: group.Name})
		}
	}
	actual, err := q.SatVerbalMappedCourses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("batched course metadata differs from existing readers:\ngot: %+v\nwant: %+v", actual, expected)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM course_merge_group_members WHERE group_id IN
		(SELECT merge_group_id FROM sat_verbal_policy_mappings WHERE merge_group_id IS NOT NULL) AND position=2`); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SatVerbalMappedCourses(ctx); err == nil || !strings.Contains(err.Error(), "exactly two source courses") {
		t.Fatalf("malformed merged mapping must fail closed, got %v", err)
	}
}

func TestSitInSubmitValidationScopesMergedTargetAndKeepsPeerSessions(t *testing.T) {
	c := seedSatCycle(t)
	c.applyAll(t)
	wcode, _ := c.student(t, "rb2")
	ctx := context.Background()
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var group, absenceID pgtype.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO course_merge_groups(name) VALUES($1) RETURNING id`, "selected-target-"+c.suffix).Scan(&group); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO course_merge_group_members(group_id,course_id,position) VALUES($1,$2,1),($1,$3,2)`, group, c.courses["rr5"].ID, c.courses["wr5"].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sat_verbal_policy_mappings SET active=false WHERE course_id=$1`, c.courses["wr5"].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sat_verbal_policy_mappings SET course_id=NULL,merge_group_id=$2 WHERE course_id=$1`, c.courses["rr5"].ID, group); err != nil {
		t.Fatal(err)
	}
	date := c.courses["rb2"].Starts[1].In(c.loc).Format("2006-01-02")
	if err := tx.QueryRow(ctx, `INSERT INTO student_absences(wcode, course_id, subject_id, sit_in_course_id, date_from, date_to)
		VALUES ($1,$2,$3,$4,$5,$5) RETURNING id`, wcode, c.courses["rb2"].ID, c.subjectID, c.courses["rr5"].ID, date).Scan(&absenceID); err != nil {
		t.Fatal(err)
	}
	db := &submitValidationDB{DBTX: tx, sessionLoads: make(map[pgtype.UUID]int)}
	q := sqldb.New(db)
	s := &server{deps: httpdeps.Deps{Q: q, InstituteTZ: "Asia/Bangkok"}}
	selected := []pgtype.UUID{makeUUID(c.courses["rr5"].Sessions[1]), makeUUID(c.courses["wr5"].Sessions[1])}
	if err := s.validateSitInCandidates(ctx, q, absenceID, selected, true); err != nil {
		t.Fatalf("eligible merged-member sessions rejected: %v", err)
	}
	if len(db.sessionLoads) != 3 {
		t.Fatalf("loaded %d course schedules, want source and both target members", len(db.sessionLoads))
	}
	if _, err := tx.Exec(ctx, `UPDATE student_absences SET sit_in_course_id=$2 WHERE id=$1`, absenceID, c.courses["rr4"].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.validateSitInCandidates(ctx, q, absenceID, selected, true); !errors.Is(err, errInvalidSitInSelection) {
		t.Fatalf("sessions outside selected target accepted: %v", err)
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
			err := s.validateSitInCandidates(ctx, f.q, absenceID, []pgtype.UUID{tc.session}, true)
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
