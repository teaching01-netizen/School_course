package absenceshttp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
)

type sessionIDDTO struct {
	ID string `json:"id"`
}

func TestFullChain_GenericPolicyAllowsFinalSitInSession(t *testing.T) {
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

	var missedFinalSessionID string
	for day := 1; day <= 10; day++ {
		start := time.Date(2026, 6, day, 9, 0, 0, 0, time.UTC)
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
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 6, 10, 12, 30, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	finalTarget, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  targetCourse.ID,
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: time.Date(2026, 6, 17, 11, 0, 0, 0, time.UTC), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: time.Date(2026, 6, 17, 12, 30, 0, 0, time.UTC), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	subjectID, _ := uuidString(subject.ID)
	courseID, _ := uuidString(missedCourse.ID)
	targetCourseID, _ := uuidString(targetCourse.ID)
	nonFinalTargetID, _ := uuidString(nonFinalTarget.ID)
	finalTargetID, _ := uuidString(finalTarget.ID)

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

	optionsResp := staffDoRequest(t, server.URL, http.MethodGet,
		"/api/v1/absences/sit-in-options?wcode="+studentWCode+"&subject_id="+subjectID+"&date_from=2026-06-10&date_to=2026-06-10",
		nil,
	)
	if optionsResp.StatusCode != http.StatusOK {
		t.Fatalf("sit-in-options status = %d", optionsResp.StatusCode)
	}
	var options struct {
		SitInMethod string `json:"sit_in_method"`
		SitInCourse *struct {
			ID string `json:"id"`
		} `json:"sit_in_course"`
		Available      []sessionIDDTO `json:"available_sessions"`
		PreSelected    []sessionIDDTO `json:"pre_selected"`
		MissedSessions []sessionIDDTO `json:"missed_sessions"`
	}
	staffParseResponse(t, optionsResp, &options)
	if options.SitInMethod != SitInMethodPhysical {
		t.Fatalf("sit-in method = %q, want %q", options.SitInMethod, SitInMethodPhysical)
	}
	if options.SitInCourse == nil || options.SitInCourse.ID != targetCourseID {
		t.Fatalf("sit-in course = %#v, want %s", options.SitInCourse, targetCourseID)
	}
	if !containsSessionID(options.Available, nonFinalTargetID) {
		t.Fatalf("available sessions = %#v, want non-final target %s", options.Available, nonFinalTargetID)
	}
	if !containsSessionID(options.Available, finalTargetID) {
		t.Fatalf("available sessions must include generic-rule final target %s: %#v", finalTargetID, options.Available)
	}
	if !containsSessionID(options.PreSelected, nonFinalTargetID) {
		t.Fatalf("pre-selected sessions = %#v, want non-final target %s", options.PreSelected, nonFinalTargetID)
	}
	if len(options.MissedSessions) != 1 || options.MissedSessions[0].ID != missedFinalSessionID {
		t.Fatalf("missed sessions = %#v, want final missed session %s", options.MissedSessions, missedFinalSessionID)
	}

	finalSubmitResp := staffDoRequest(t, server.URL, http.MethodPost, "/api/v1/absences", map[string]any{
		"wcode":              studentWCode,
		"subject_id":         subjectID,
		"course_id":          courseID,
		"date_from":          "2026-06-10",
		"date_to":            "2026-06-10",
		"sit_in_method":      SitInMethodPhysical,
		"sit_in_course_id":   targetCourseID,
		"missed_session_ids": []string{missedFinalSessionID},
		"sit_in_session_ids": []string{finalTargetID},
	})
	if finalSubmitResp.StatusCode != http.StatusCreated {
		t.Fatalf("final sit-in submit status = %d, want 201", finalSubmitResp.StatusCode)
	}
	var created map[string]any
	staffParseResponse(t, finalSubmitResp, &created)
	if created["sit_in_method"] != SitInMethodPhysical {
		t.Fatalf("created response = %#v, want physical sit-in", created)
	}
}

func containsSessionID(items []sessionIDDTO, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
