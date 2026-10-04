package absenceshttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/absences/sitinresolver"
	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/satverbalpolicy"
	"warwick-institute/internal/studentauth"
)

func (f finalSessionFixture) staffServer(t *testing.T) *httptest.Server {
	t.Helper()
	admin, err := f.q.AdminUserCreate(context.Background(), sqldb.AdminUserCreateParams{
		Username: "final-admin-" + f.courseID, Role: "Admin", PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Q: f.q, DB: f.pool, Log: slog.Default(), InstituteTZ: "Asia/Bangkok",
		Auth:          staffFakeAuth{user: auth.AuthenticatedUser{ID: uuid.UUID(admin.Bytes), Role: "Admin"}},
		SitInResolver: sitinresolver.New(f.q, "Asia/Bangkok"),
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func (f finalSessionFixture) requestBody() map[string]any {
	return map[string]any{
		"wcode": f.wcode, "subject_id": f.subjectID, "course_id": f.courseID,
		"date_from": f.date, "date_to": f.date, "reason": "Medical appointment",
		"sit_in_method": SitInMethodPhysical, "sit_in_course_id": f.targetCourseID,
		"missed_session_ids": []string{f.missedID}, "sit_in_session_ids": []string{f.finalID},
	}
}

func assertFinalSessionResponse(t *testing.T, status, wantStatus int, body []byte) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", status, wantStatus, body)
	}
}

func finalSessionResponseBody(t *testing.T, response *http.Response, wantStatus int) []byte {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	assertFinalSessionResponse(t, response.StatusCode, wantStatus, body)
	return body
}

func (f finalSessionFixture) assertPersisted(t *testing.T, sitInID string) string {
	t.Helper()
	var absenceID, missedID, assignedID string
	if err := f.pool.QueryRow(context.Background(), `
		SELECT sa.id, ams.session_id, asi.session_id
		FROM student_absences sa
		JOIN absence_missed_sessions ams ON ams.absence_id = sa.id
		JOIN absence_sit_ins asi ON asi.absence_id = sa.id
		WHERE lower(sa.wcode) = lower($1)
	`, f.wcode).Scan(&absenceID, &missedID, &assignedID); err != nil {
		t.Fatal(err)
	}
	if missedID != f.missedID || assignedID != sitInID {
		t.Fatalf("saved missed/sit-in = %s/%s, want %s/%s", missedID, assignedID, f.missedID, sitInID)
	}
	return absenceID
}

func (f finalSessionFixture) assertDiscovery(t *testing.T, body []byte, finalAllowed bool) {
	t.Helper()
	var result struct {
		Subjects []courseJSON `json:"subjects"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Subjects) != 1 || len(result.Subjects[0].Sessions) != 1 || result.Subjects[0].Sessions[0].ID != f.missedID {
		t.Fatalf("final leave session missing: %s", body)
	}
	options := result.Subjects[0].SitIn
	if options == nil {
		t.Fatalf("missing sit-in options: %s", body)
	}
	available := options.AvailableSessions
	for _, priority := range options.Priorities {
		available = append(available, priority.Available...)
	}
	seen := make(map[string]bool)
	for _, session := range available {
		seen[session.ID] = true
	}
	if !seen[f.earlierID] || seen[f.finalID] != finalAllowed {
		t.Fatalf("sit-in availability = %s; want earlier=true final=%v", body, finalAllowed)
	}
}

func TestFullChain_GenericPolicyAllowsFinalSitInSession(t *testing.T) {
	for _, inactiveMapping := range []bool{false, true} {
		mappingName := "unmapped"
		if inactiveMapping {
			mappingName = "inactive_mapping"
		}
		for _, flow := range []string{"single", "student_batch", "staff_form"} {
			t.Run(mappingName+"/"+flow, func(t *testing.T) {
				f := seedFinalSessionFixture(t)
				if inactiveMapping {
					if _, err := f.pool.Exec(context.Background(), `
						INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, policy_rule, policy_hash, active)
						VALUES ($1, $2, '{"id":"inactive","courseName":"SAT Verbal Rank 2","lastClassExcluded":true}', '', false)
					`, "inactive-"+f.courseID, f.courseID); err != nil {
						t.Fatal(err)
					}
				}
				staff := f.staffServer(t)
				body := f.requestBody()
				optionsResponse := staffDoRequest(t, staff.URL, http.MethodGet,
					"/api/v1/absences/sit-in-options?wcode="+f.wcode+"&subject_id="+f.subjectID+"&date_from="+f.date+"&date_to="+f.date, nil)
				var options SitInResult
				if err := json.Unmarshal(finalSessionResponseBody(t, optionsResponse, http.StatusOK), &options); err != nil {
					t.Fatal(err)
				}
				if len(options.MissedSession) != 1 || options.MissedSession[0].ID != f.missedID || len(options.Available) != 2 {
					t.Fatalf("legacy sit-in options = %#v, want final missed session and both target sessions", options)
				}
				if flow == "student_batch" {
					mux := selfServiceMux(t, f.pool)
					token := seedVerifiedStudentSession(t, f.pool, f.wcode)
					request := httptest.NewRequest(http.MethodGet, "/api/v1/absence-self-service/sessions?date_from="+f.date+"&date_to="+f.date, nil)
					request.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: token})
					recorder := httptest.NewRecorder()
					mux.ServeHTTP(recorder, request)
					assertFinalSessionResponse(t, recorder.Code, http.StatusOK, recorder.Body.Bytes())
					f.assertDiscovery(t, recorder.Body.Bytes(), true)
					delete(body, "wcode")
					recorder = postSelfService(t, mux, "/api/v1/absences/batch", token, map[string]any{
						"reason": "Medical appointment", "items": []map[string]any{body},
					})
					assertFinalSessionResponse(t, recorder.Code, http.StatusCreated, recorder.Body.Bytes())
				} else {
					response := staffDoRequest(t, staff.URL, http.MethodGet,
						"/api/v1/absences/sessions-in-range?wcode="+f.wcode+"&date_from="+f.date+"&date_to="+f.date+"&student_view=true", nil)
					f.assertDiscovery(t, finalSessionResponseBody(t, response, http.StatusOK), true)
					path := "/api/v1/absences"
					if flow == "staff_form" {
						path = "/api/v1/absences/staff-form-batch"
						body = map[string]any{"items": []map[string]any{body}}
					}
					response = staffDoRequest(t, staff.URL, http.MethodPost, path, body)
					finalSessionResponseBody(t, response, http.StatusCreated)
				}
				absenceID := f.assertPersisted(t, f.finalID)
				response := staffDoRequest(t, staff.URL, http.MethodPost, "/api/v1/absences", f.requestBody())
				conflictBody := finalSessionResponseBody(t, response, http.StatusConflict)
				var conflict struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(conflictBody, &conflict); err != nil {
					t.Fatal(err)
				}
				if conflict.Code != "sit_in_session_already_used" {
					t.Fatalf("conflict = %s", conflictBody)
				}
				for _, sitInID := range []string{f.earlierID, f.finalID} {
					var version int32
					if err := f.pool.QueryRow(context.Background(), "SELECT version FROM student_absences WHERE id = $1", absenceID).Scan(&version); err != nil {
						t.Fatal(err)
					}
					response := staffDoRequest(t, staff.URL, http.MethodPut, "/api/v1/absences/"+absenceID+"/sit-in", map[string]any{
						"method": "physical", "sit_in_course_id": f.targetCourseID, "sit_in_session_ids": []string{sitInID},
						"expected_version": version, "reason": "Change make-up time",
					})
					finalSessionResponseBody(t, response, http.StatusOK)
					f.assertPersisted(t, sitInID)
				}
			})
		}
	}
}

func TestFullChain_FinalSitInRespectsConfiguredWindow(t *testing.T) {
	f := seedFinalSessionFixture(t)
	ctx := context.Background()
	var rootID string
	if err := f.pool.QueryRow(ctx, "SELECT root_course_group_id FROM courses WHERE id = $1", f.courseID).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	policies, err := json.Marshal(sqldb.AbsencePolicies{RootCourseGroups: map[string]sqldb.SubjectPolicy{rootID: {SitInWindowWeeks: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.q.AppSettingsUpdateAbsencePolicies(ctx, policies); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.q.AppSettingsUpdateAbsencePolicies(ctx, []byte(`{}`)); err != nil {
			t.Error(err)
		}
	})
	staff := f.staffServer(t)
	response := staffDoRequest(t, staff.URL, http.MethodGet,
		"/api/v1/absences/sessions-in-range?wcode="+f.wcode+"&date_from="+f.date+"&date_to="+f.date+"&student_view=true", nil)
	f.assertDiscovery(t, finalSessionResponseBody(t, response, http.StatusOK), false)
}

func TestFullChain_UnmappedFinalSitInIgnoresAnotherCoursesWindow(t *testing.T) {
	f := seedFinalSessionFixture(t)
	other := seedFinalSessionFixture(t)
	ctx := context.Background()
	student, err := f.q.StudentGetByWCode(ctx, f.wcode)
	if err != nil {
		t.Fatal(err)
	}
	// Enroll without overlapping the first course's normal timetable.
	if _, err := other.pool.Exec(ctx, "UPDATE sessions SET start_at = start_at + interval '4 hours', end_at = end_at + interval '4 hours' WHERE course_id = $1", other.courseID); err != nil {
		t.Fatal(err)
	}
	if err := f.q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{CourseID: makeUUID(other.courseID), StudentID: student.ID}); err != nil {
		t.Fatal(err)
	}
	var rootID string
	if err := f.pool.QueryRow(ctx, "SELECT root_course_group_id FROM courses WHERE id = $1", other.courseID).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	policies, err := json.Marshal(sqldb.AbsencePolicies{RootCourseGroups: map[string]sqldb.SubjectPolicy{rootID: {SitInWindowWeeks: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.q.AppSettingsUpdateAbsencePolicies(ctx, policies); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.q.AppSettingsUpdateAbsencePolicies(ctx, []byte(`{}`)); err != nil {
			t.Error(err)
		}
	})
	staff := f.staffServer(t)
	response := staffDoRequest(t, staff.URL, http.MethodGet,
		"/api/v1/absences/sessions-in-range?wcode="+f.wcode+"&course_ids="+f.courseID+"&date_from="+f.date+"&date_to="+f.date+"&student_view=true", nil)
	f.assertDiscovery(t, finalSessionResponseBody(t, response, http.StatusOK), true)
}

func TestFullChain_FinalSessionsKeepStudentValidation(t *testing.T) {
	for _, scenario := range []struct {
		name, code string
		status     int
	}{
		{"unenrolled", "not_found", http.StatusNotFound},
		{"deleted", "invalid_sessions", http.StatusBadRequest},
		{"hidden", "sit_in_course_inactive", http.StatusBadRequest},
		{"overlap", "invalid_sessions", http.StatusBadRequest},
		{"absence_limit", "absence_limit_exceeded", http.StatusForbidden},
		{"session_limit", "too_many_sessions", http.StatusBadRequest},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := seedFinalSessionFixture(t)
			ctx := context.Background()
			body := f.requestBody()
			delete(body, "wcode")
			var err error
			existingAbsences := 0
			switch scenario.name {
			case "unenrolled":
				_, err = f.pool.Exec(ctx, "DELETE FROM course_students WHERE course_id = $1", f.courseID)
			case "deleted":
				_, err = f.pool.Exec(ctx, "UPDATE sessions SET deleted_at = now() WHERE id = $1", f.finalID)
			case "hidden":
				_, err = f.pool.Exec(ctx, "UPDATE courses SET absence_form_visible = false WHERE id = $1", f.targetCourseID)
			case "overlap":
				teacher, createErr := f.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{Username: "overlap-" + f.courseID, Role: "Teacher", PasswordHash: "x"})
				if createErr != nil {
					t.Fatal(createErr)
				}
				if _, err := f.pool.Exec(ctx, "UPDATE sessions SET start_at = start_at - interval '1 day', end_at = end_at - interval '1 day' WHERE id = $1", f.earlierID); err != nil {
					t.Fatal(err)
				}
				_, err = f.pool.Exec(ctx, `
					UPDATE sessions SET start_at = missed.start_at + interval '30 minutes', end_at = missed.end_at, teacher_id = $3
					FROM sessions missed WHERE sessions.id = $1 AND missed.id = $2
				`, f.finalID, f.missedID, teacher)
			case "absence_limit":
				existingAbsences = 2
				_, err = f.pool.Exec(ctx, `
					INSERT INTO student_absences (wcode, course_id, date_from, date_to, status)
					SELECT $1, course_id, (start_at AT TIME ZONE 'Asia/Bangkok')::date, (start_at AT TIME ZONE 'Asia/Bangkok')::date, 'pending'
					FROM sessions WHERE course_id = $2 ORDER BY start_at LIMIT 2
				`, f.wcode, f.courseID)
			case "session_limit":
				err = f.q.AppSettingsUpdateAbsencePolicies(ctx, []byte(`{"sit_in":{"max_sessions_per_absence":1}}`))
				t.Cleanup(func() {
					if err := f.q.AppSettingsUpdateAbsencePolicies(ctx, []byte(`{}`)); err != nil {
						t.Error(err)
					}
				})
				body["sit_in_session_ids"] = []string{f.earlierID, f.finalID}
			}
			if err != nil {
				t.Fatal(err)
			}
			mux := selfServiceMux(t, f.pool)
			token := seedVerifiedStudentSession(t, f.pool, f.wcode)
			recorder := postSelfService(t, mux, "/api/v1/absences/batch", token, map[string]any{
				"reason": "Medical appointment", "items": []map[string]any{body},
			})
			assertFinalSessionResponse(t, recorder.Code, scenario.status, recorder.Body.Bytes())
			var rejection struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &rejection); err != nil {
				t.Fatal(err)
			}
			if rejection.Code != scenario.code {
				t.Fatalf("rejection = %s, want %s", recorder.Body.String(), scenario.code)
			}
			var count int
			if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM student_absences WHERE lower(wcode) = lower($1)", f.wcode).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != existingAbsences {
				t.Fatalf("rejection persisted absences: count=%d, want %d", count, existingAbsences)
			}
		})
	}
}

func TestFullChain_FinalSitInFollowsActiveSatVerbalMapping(t *testing.T) {
	for _, merged := range []bool{false, true} {
		for _, excluded := range []bool{false, true} {
			name := "direct/allowed"
			if merged {
				name = "merge_group/allowed"
			}
			if excluded {
				name = strings.Replace(name, "allowed", "excluded", 1)
			}
			t.Run(name, func(t *testing.T) {
				f := seedFinalSessionFixture(t)
				ctx := context.Background()
				t.Cleanup(func() {
					if _, err := f.pool.Exec(ctx, "DELETE FROM sat_verbal_policy_mappings WHERE rule_id IN ($1, $2)", "source-"+f.courseID, "target-"+f.targetCourseID); err != nil {
						t.Error(err)
					}
				})
				targetName := "Final Target " + f.targetCourseID
				rule := satverbalpolicy.CourseRule{
					ID: "source-" + f.courseID, CourseName: "Final Source " + f.courseID,
					LastClassExcluded: excluded,
					Priorities: []satverbalpolicy.RulePriority{{
						Level: 1, RuleType: RuleTypeAnyDayExceptLast, Label: "Any target session",
						AnyDay: true, EligibleTargets: []string{targetName},
					}},
				}
				var courseID any = f.courseID
				var mergeID any
				if merged {
					group, err := f.q.CourseMergeGroupCreate(ctx, "Final policy "+f.courseID, f.teacherID)
					if err != nil {
						t.Fatal(err)
					}
					if err := f.q.CourseMergeGroupAssignCourse(ctx, group.ID, makeUUID(f.courseID), 1); err != nil {
						t.Fatal(err)
					}
					peer, err := f.q.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "PEER-" + f.courseID, Name: "Merged source peer"})
					if err != nil {
						t.Fatal(err)
					}
					if _, err := f.pool.Exec(ctx, "UPDATE courses SET subject_id = $1 WHERE id = $2", f.subjectID, peer.ID); err != nil {
						t.Fatal(err)
					}
					if err := f.q.CourseMergeGroupAssignCourse(ctx, group.ID, peer.ID, 2); err != nil {
						t.Fatal(err)
					}
					courseID = nil
					mergeID = group.ID
				}
				for _, mapping := range []struct {
					rule          satverbalpolicy.CourseRule
					course, merge any
				}{
					{rule, courseID, mergeID},
					{satverbalpolicy.CourseRule{ID: "target-" + f.targetCourseID, CourseName: targetName}, f.targetCourseID, nil},
				} {
					raw, err := json.Marshal(mapping.rule)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := f.pool.Exec(ctx, `
						INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, merge_group_id, policy_rule, policy_hash, active)
						VALUES ($1, $2, $3, $4, '', true)
					`, mapping.rule.ID, mapping.course, mapping.merge, string(raw)); err != nil {
						t.Fatal(err)
					}
				}
				staff := f.staffServer(t)
				response := staffDoRequest(t, staff.URL, http.MethodGet,
					"/api/v1/absences/sessions-in-range?wcode="+f.wcode+"&date_from="+f.date+"&date_to="+f.date+"&student_view=true", nil)
				f.assertDiscovery(t, finalSessionResponseBody(t, response, http.StatusOK), !excluded)
				body := f.requestBody()
				if excluded {
					response = staffDoRequest(t, staff.URL, http.MethodPost, "/api/v1/absences", body)
					finalSessionResponseBody(t, response, http.StatusBadRequest)
					var count int
					if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM student_absences WHERE lower(wcode) = lower($1)", f.wcode).Scan(&count); err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Fatalf("rejected final session persisted %d absences", count)
					}
				}
				body["sit_in_session_ids"] = []string{f.earlierID}
				response = staffDoRequest(t, staff.URL, http.MethodPost, "/api/v1/absences", body)
				finalSessionResponseBody(t, response, http.StatusCreated)
				absenceID := f.assertPersisted(t, f.earlierID)
				var version int32
				if err := f.pool.QueryRow(ctx, "SELECT version FROM student_absences WHERE id = $1", absenceID).Scan(&version); err != nil {
					t.Fatal(err)
				}
				response = staffDoRequest(t, staff.URL, http.MethodPut, "/api/v1/absences/"+absenceID+"/sit-in", map[string]any{
					"method": "physical", "sit_in_course_id": f.targetCourseID, "sit_in_session_ids": []string{f.finalID},
					"expected_version": version, "reason": "Request final make-up session",
				})
				wantStatus := http.StatusOK
				if excluded {
					wantStatus = http.StatusBadRequest
				}
				finalSessionResponseBody(t, response, wantStatus)
				assignedID := f.finalID
				if excluded {
					assignedID = f.earlierID
				}
				f.assertPersisted(t, assignedID)
			})
		}
	}
}

type finalSessionFixture struct {
	q                                          *sqldb.Queries
	pool                                       *pgxpool.Pool
	wcode, subjectID, courseID, targetCourseID string
	missedID, earlierID, finalID, date         string
	teacherID                                  pgtype.UUID
}

func seedFinalSessionFixture(t *testing.T) finalSessionFixture {
	t.Helper()
	databaseURL := requireStaffTestDB(t)
	migrateStaffUpOnce(t, databaseURL)
	dbpool := newStaffPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := sqldb.New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	settingsJSON := []byte(`{}`)
	if err := q.AppSettingsUpdateAbsencePolicies(ctx, settingsJSON); err != nil {
		t.Fatal(err)
	}

	suffix := strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	subject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: "QAFS-" + suffix,
		Name: "QA Final Sit-In " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := q.SitInRuleCreate(ctx, sqldb.SitInRuleCreateInput{
		Name: "QA final target exclusion " + suffix,
		Type: RuleTypeLevelLadder,
		Predicate: json.RawMessage(`{
			"level_1_action": "zoom",
			"non_max_direction": "higher",
			"max_direction": "lower",
			"min_level_for_sit_lower": 2,
			"section_match": "same_section",
			"occurrence_match": "any",
			"day_match": "any",
			"last_class_excluded": true,
			"schedule_source": "target",
			"chains": [],
			"auto_assign": true,
			"requires_teacher_approval": false
		}`),
		Description: "QA full-chain final sit-in exclusion",
	})
	if err != nil {
		t.Fatal(err)
	}
	rootID, _, _, err := q.RootCourseGroupCreate(ctx, "QA final sit-in group "+suffix, rule.ID)
	if err != nil {
		t.Fatal(err)
	}

	missedCourse, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "QAFSM-" + suffix,
		Name: "QA Final Missed " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	targetCourse, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "QAFST-" + suffix,
		Name: "QA Final Target " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	cycleID := pgtype.Text{String: "QA-FINAL-" + suffix, Valid: true}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO crm_cycles (id, label) VALUES ($1, $2)
	`, cycleID.String, "QA Final Cycle "+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		UPDATE courses
		SET subject_id = $1, cycle_id = $2, root_course_group_id = $3, level = $4
		WHERE id = $5
	`, subject.ID, cycleID, rootID, int16(2), missedCourse.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		UPDATE courses
		SET subject_id = $1, cycle_id = $2, root_course_group_id = $3, level = $4
		WHERE id = $5
	`, subject.ID, cycleID, rootID, int16(3), targetCourse.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpool.Exec(ctx, `
		INSERT INTO subject_active_courses (subject_id, course_id)
		VALUES ($1, $2), ($1, $3)
	`, subject.ID, missedCourse.ID, targetCourse.ID); err != nil {
		t.Fatal(err)
	}

	studentWCode := "wqafs" + suffix
	student, err := q.StudentCreate(ctx, sqldb.StudentCreateParams{
		Wcode:    studentWCode,
		FullName: "QA Final Sit-In Student " + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CourseStudentAdd(ctx, sqldb.CourseStudentAddParams{
		CourseID:  missedCourse.ID,
		StudentID: student.ID,
	}); err != nil {
		t.Fatal(err)
	}

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "qafs-teacher-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}

	base := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 7).Add(9 * time.Hour)
	var missedFinalSessionID string
	for day := 1; day <= 10; day++ {
		start := base.AddDate(0, 0, day-1)
		session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:  missedCourse.ID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: start.Add(90 * time.Minute), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if day == 10 {
			missedFinalSessionID, _ = uuidString(session.ID)
		}
	}

	nonFinalTarget, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  targetCourse.ID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: base.AddDate(0, 0, 9).Add(2 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: base.AddDate(0, 0, 9).Add(210 * time.Minute), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	finalTarget, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  targetCourse.ID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: base.AddDate(0, 0, 16).Add(2 * time.Hour), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: base.AddDate(0, 0, 16).Add(210 * time.Minute), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	subjectID, _ := uuidString(subject.ID)
	courseID, _ := uuidString(missedCourse.ID)
	targetCourseID, _ := uuidString(targetCourse.ID)
	nonFinalTargetID, _ := uuidString(nonFinalTarget.ID)
	finalTargetID, _ := uuidString(finalTarget.ID)

	return finalSessionFixture{
		q: q, pool: dbpool, wcode: studentWCode, subjectID: subjectID, courseID: courseID,
		targetCourseID: targetCourseID, missedID: missedFinalSessionID,
		earlierID: nonFinalTargetID, finalID: finalTargetID,
		date: base.AddDate(0, 0, 9).Format("2006-01-02"), teacherID: teacherID,
	}
}
