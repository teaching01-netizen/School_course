package absenceshttp

// Audit reproduction: copy into this package as zz_policy_audit_test.go.
// These assertions state the intended safety behavior and fail on the audited code.

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"warwick-institute/internal/absences/sitinresolver"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type policyAuditFixture struct {
	q                                                                                *sqldb.Queries
	tx                                                                               pgx.Tx
	student, subject, missed, sibling, target, other, root, merge                    pgtype.UUID
	missedSession, secondMissed, overlapping, adjacent, later, concurrent, unrelated pgtype.UUID
	wcode                                                                            string
	day                                                                              time.Time
}

func seedPolicyAudit(t *testing.T) policyAuditFixture {
	t.Helper()
	u := requireTestDBPending(t)
	migrateUpOncePending(t, u)
	pool := newPoolPending(t, u)
	t.Cleanup(pool.Close)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	f := policyAuditFixture{q: sqldb.New(pool).WithTx(tx), tx: tx, wcode: "waudit-" + uuid.NewString()[:8], day: time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour)}
	scan := func(dst any, query string, args ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, query, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	scan(&f.student, `INSERT INTO students(wcode,full_name) VALUES($1,'Policy audit') RETURNING id`, f.wcode)
	scan(&f.subject, `INSERT INTO subjects(code,name) VALUES($1,'Audit source') RETURNING id`, f.wcode+"-subject")
	var siblingSubject, otherSubject, ladder pgtype.UUID
	scan(&siblingSubject, `INSERT INTO subjects(code,name) VALUES($1,'Audit sibling') RETURNING id`, f.wcode+"-sibling")
	scan(&otherSubject, `INSERT INTO subjects(code,name) VALUES($1,'Audit other') RETURNING id`, f.wcode+"-other")
	scan(&ladder, `INSERT INTO sit_in_rules(name,type,predicate) VALUES($1,'level_ladder','{}') RETURNING id`, f.wcode+"-ladder")
	scan(&f.root, `INSERT INTO root_course_groups(name) VALUES($1) RETURNING id`, f.wcode+"-root")
	newCourse := func(dst *pgtype.UUID, subject pgtype.UUID, level int, root pgtype.UUID) {
		scan(dst, `INSERT INTO courses(code,name,subject_id,level,root_course_group_id) VALUES($1,$1,$2,$3,$4) RETURNING id`, f.wcode+"-"+uuid.NewString()[:8], subject, level, root)
		exec(`INSERT INTO subject_active_courses(subject_id,course_id) VALUES($1,$2)`, subject, *dst)
	}
	newCourse(&f.missed, f.subject, 2, f.root)
	newCourse(&f.target, f.subject, 3, f.root)
	newCourse(&f.sibling, siblingSubject, 2, pgtype.UUID{})
	newCourse(&f.other, otherSubject, 2, pgtype.UUID{})
	for _, c := range []pgtype.UUID{f.missed, f.sibling} {
		exec(`INSERT INTO course_students(course_id,student_id,status) VALUES($1,$2,'enrolled')`, c, f.student)
	}
	scan(&f.merge, `INSERT INTO course_merge_groups(name) VALUES($1) RETURNING id`, f.wcode+"-merge")
	for i, c := range []pgtype.UUID{f.missed, f.sibling} {
		exec(`INSERT INTO course_merge_group_members(group_id,course_id,position) VALUES($1,$2,$3)`, f.merge, c, i+1)
	}
	exec(`INSERT INTO sit_in_priorities(root_course_group_id,sit_in_rule_id,priority_level,label) VALUES($1,$2,1,'Audit priority')`, f.root, ladder)
	add := func(dst *pgtype.UUID, c pgtype.UUID, start time.Time) {
		var teacher pgtype.UUID
		scan(&teacher, `INSERT INTO users(username,role,password_hash) VALUES($1,'Teacher','x') RETURNING id`, f.wcode+"-"+uuid.NewString()[:8])
		scan(dst, `INSERT INTO sessions(course_id,teacher_id,start_at,end_at) VALUES($1,$2,$3,$4) RETURNING id`, c, teacher, start, start.Add(time.Hour))
	}
	add(&f.missedSession, f.missed, f.day.Add(15*time.Hour))
	add(&f.secondMissed, f.missed, f.day.Add(39*time.Hour))
	var siblingSession pgtype.UUID
	add(&siblingSession, f.sibling, f.day.Add(13*time.Hour))
	add(&f.overlapping, f.target, f.day.Add(13*time.Hour))
	add(&f.adjacent, f.target, f.day.Add(14*time.Hour))
	add(&f.later, f.target, f.day.Add(37*time.Hour))
	add(&f.concurrent, f.target, f.day.Add(37*time.Hour+30*time.Minute))
	add(&f.unrelated, f.other, f.day.Add(41*time.Hour))
	return f
}

func (f policyAuditFixture) v2(t *testing.T, lookup sessionsRangeLookup, policies []byte, settings absenceSettings, now time.Time) *courseSitInJSON {
	t.Helper()
	ctx := context.Background()
	rows, err := f.q.SessionsRangeFacts(ctx, sqldb.SessionsRangeFactsParams{Wcode: f.wcode, FromUTC: f.day, ToExclusiveUTC: f.day.Add(24 * time.Hour), Mode: sqldb.SessionsRangeFactsStudent, InstituteTZ: "Asia/Bangkok"})
	if err != nil {
		t.Fatal(err)
	}
	facts := applyTimingAndCourseFilter(normalizeSessionFacts(rows, "Asia/Bangkok"), lookup, settings, now)
	bundle, err := f.q.SessionsRangeSitInBundleV2(ctx, sqldb.SitInBundleV2Params{StudentID: f.student, MissedCourseIDs: []pgtype.UUID{f.missed}, PoliciesJSON: policies, NowUTC: now})
	if err != nil {
		t.Fatal(err)
	}
	s := &server{deps: httpdeps.Deps{Q: f.q, InstituteTZ: "Asia/Bangkok"}, a: httpadapter.New(nil, nil)}
	for _, g := range groupFactsByCourse(facts) {
		if g.courseID == uuidStringOrZero(f.missed) {
			return s.resolveEnrolledCourseSitInV2(g, facts, bundle, policies, nil, nil, true, sessionsRangePrelim{dateFrom: f.day, dateTo: f.day}, lookup, f.wcode, f.student, now)
		}
	}
	t.Fatal("audit source class missing from fixture response")
	return nil
}

func TestPolicyAuditV2KeepsAuthoritativeBlockers(t *testing.T) {
	for _, kind := range []string{"course_allow_list", "timing_policy", "hidden_expected_class"} {
		t.Run(kind, func(t *testing.T) {
			f := seedPolicyAudit(t)
			settings := defaultAbsenceSettings()
			lookup := StudentSessionLookup{WCode: f.wcode}
			now := f.day.Add(-24 * time.Hour)
			if kind == "course_allow_list" {
				lookup.CourseIDs = map[string]bool{uuidStringOrZero(f.missed): true}
			}
			if kind == "timing_policy" {
				settings.Form.MinHoursBeforeSession = 26
				now = f.day.Add(-12 * time.Hour)
			}
			if kind == "hidden_expected_class" {
				if _, err := f.tx.Exec(context.Background(), `UPDATE courses SET absence_form_visible=false WHERE id=$1`, f.sibling); err != nil {
					t.Fatal(err)
				}
			}
			blockers, err := f.q.SitInConflictSessionsForStudent(context.Background(), f.student, f.missed, f.day, f.day, "Asia/Bangkok")
			if err != nil {
				t.Fatal(err)
			}
			if !expectedSessionOverlap(sessionBrief{StartAt: f.day.Add(13 * time.Hour).Format(time.RFC3339), EndAt: f.day.Add(14 * time.Hour).Format(time.RFC3339)}, blockers) {
				t.Fatal("fixture lacks authoritative conflict")
			}
			result := f.v2(t, lookup, []byte(`{}`), settings, now)
			if result == nil {
				t.Fatal("missing policy result")
			}
			for _, p := range result.Priorities {
				for _, c := range p.Available {
					if c.ID == uuidStringOrZero(f.overlapping) {
						t.Fatalf("V2 offers overlapping choice after %s hides the blocker", kind)
					}
				}
			}
		})
	}
}

func TestPolicyAuditAutomaticPolicyDisabled(t *testing.T) {
	for _, kind := range []string{"global", "root_group", "merge_group"} {
		t.Run(kind, func(t *testing.T) {
			f := seedPolicyAudit(t)
			policies := `{"sit_in":{"auto_resolve_enabled":false}}`
			if kind == "root_group" {
				policies = fmt.Sprintf(`{"root_course_groups":{"%s":{"auto_sit_in_enabled":false}}}`, uuidStringOrZero(f.root))
			}
			if kind == "merge_group" {
				policies = fmt.Sprintf(`{"merge_groups":{"%s":{"auto_sit_in_enabled":false}}}`, uuidStringOrZero(f.merge))
			}
			if result := f.v2(t, StudentSessionLookup{WCode: f.wcode}, []byte(policies), defaultAbsenceSettings(), f.day.Add(-24*time.Hour)); result != nil {
				t.Fatalf("automatic policy %s is disabled but resolver still returns make-up options", kind)
			}
		})
	}
}

func TestPolicyAuditPickerChecksClassesOnMakeUpDate(t *testing.T) {
	f := seedPolicyAudit(t)
	ctx := context.Background()
	if _, err := f.tx.Exec(ctx, `INSERT INTO course_students(course_id,student_id,status) VALUES($1,$2,'enrolled')`, f.other, f.student); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tx.Exec(ctx, `UPDATE sessions SET start_at=$2,end_at=$3 WHERE id=$1`, f.unrelated, f.day.Add(37*time.Hour), f.day.Add(38*time.Hour)); err != nil {
		t.Fatal(err)
	}
	validation, err := sitinresolver.New(f.q, "Asia/Bangkok").ValidateCandidate(ctx, f.createAuditAbsence(t), f.later)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("fixture has no submission-time regular-class conflict")
	}
	result := f.v2(t, StudentSessionLookup{WCode: f.wcode}, []byte(`{}`), defaultAbsenceSettings(), f.day.Add(-24*time.Hour))
	if result == nil {
		t.Fatal("missing policy result")
	}
	for _, priority := range result.Priorities {
		for _, candidate := range priority.Available {
			if candidate.ID == uuidStringOrZero(f.later) {
				t.Fatal("picker offers a make-up overlapping an expected class outside the absence date window; final validator rejects it")
			}
		}
	}
}

func (f policyAuditFixture) createAuditAbsence(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := f.tx.QueryRow(context.Background(), `INSERT INTO student_absences(wcode,course_id,date_from,date_to) VALUES($1,$2,$3,$3) RETURNING id`, f.wcode, f.missed, f.day.Format("2006-01-02")).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPolicyAuditGenericFinalDayExcluded(t *testing.T) {
	f := seedPolicyAudit(t)
	ctx := context.Background()
	var rule pgtype.UUID
	if err := f.tx.QueryRow(ctx, `INSERT INTO sit_in_rules(name,type,predicate) VALUES($1,'any_day_except_last','{"last_class_excluded":true}') RETURNING id`, f.wcode+"-any").Scan(&rule); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tx.Exec(ctx, `DELETE FROM sit_in_priorities WHERE root_course_group_id=$1`, f.root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.tx.Exec(ctx, `UPDATE root_course_groups SET sit_in_rule_id=$2 WHERE id=$1`, f.root, rule); err != nil {
		t.Fatal(err)
	}
	result, err := resolveSitInForCourse(ctx, f.q, f.wcode, f.missed, f.subject, f.day, f.day.Add(24*time.Hour), "Asia/Bangkok", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("missing generic policy result")
	}
	for _, c := range result.Available {
		if c.ID == uuidStringOrZero(f.later) || c.ID == uuidStringOrZero(f.concurrent) {
			t.Fatal("any_day_except_last offers a session on the target's final day")
		}
	}
}

func (f policyAuditFixture) submit(t *testing.T, missed, candidate, target pgtype.UUID, date time.Time) bool {
	t.Helper()
	s := &server{deps: httpdeps.Deps{Q: f.q, InstituteTZ: "Asia/Bangkok", SitInResolver: sitinresolver.New(f.q, "Asia/Bangkok")}, a: httpadapter.New(nil, nil)}
	settings := defaultAbsenceSettings()
	settings.Form.AbsenceLimitPercent = 100
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/audit", nil)
	method, course := "physical", uuidStringOrZero(target)
	_, ok := s.createAbsenceRecordTx(w, r, f.q, f.tx, settings, f.wcode, pgtype.Text{}, pgtype.Text{String: "audit", Valid: true}, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, batchAbsenceCreateItem{SubjectID: uuidStringOrZero(f.subject), CourseID: uuidStringOrZero(f.missed), DateFrom: date.Format("2006-01-02"), DateTo: date.Format("2006-01-02"), SitInMethod: &method, SitInCourseID: &course, MissedSessionIDs: []string{uuidStringOrZero(missed)}, SitInSessionIDs: []string{uuidStringOrZero(candidate)}})
	if !ok {
		t.Logf("submission rejected: %s", w.Body.String())
	}
	return ok
}

func TestPolicyAuditDifferentMakeUpsCannotOverlap(t *testing.T) {
	f := seedPolicyAudit(t)
	if !f.submit(t, f.missedSession, f.later, f.target, f.day) {
		t.Fatal("first valid fixture submission rejected")
	}
	if f.submit(t, f.secondMissed, f.concurrent, f.target, f.day.Add(24*time.Hour)) {
		t.Fatal("two different overlapping make-up sessions accepted for the same student")
	}
}

func TestPolicyAuditSessionMustMatchAllowedTarget(t *testing.T) {
	f := seedPolicyAudit(t)
	if f.submit(t, f.missedSession, f.unrelated, f.target, f.day) {
		t.Fatal("submission accepted a session from an unrelated class outside the declared policy target")
	}
}

func TestPolicyAuditCrossSectionChoosesAnotherCourse(t *testing.T) {
	missed := makeUUID("aaaaaaaa-0000-0000-0000-000000000001")
	sibling := makeUUID("aaaaaaaa-0000-0000-0000-000000000002")
	result, err := EvaluateRule(EvaluateRuleInput{
		RuleType: RuleTypeCrossSection, StudentLevel: 2,
		AllCourses: []sqldb.SubjectCourseV2{
			{ID: missed, Level: pgtype.Int2{Int16: 2, Valid: true}},
			{ID: sibling, Level: pgtype.Int2{Int16: 2, Valid: true}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetCourseID == nil || *result.TargetCourseID == missed {
		t.Fatal("cross-section policy selects the missed course itself instead of the sibling course")
	}
}
