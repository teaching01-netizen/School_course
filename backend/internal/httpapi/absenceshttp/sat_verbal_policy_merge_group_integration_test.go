package absenceshttp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/satverbalpolicy"
)

func TestResolveMappedSatVerbalSitIn_MergedMissedCourseOffersNonMergedTarget(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	q := sqldb.New(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := q.WithTx(tx)

	suffix := strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	teacherID, err := qtx.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "sat-verbal-merge-sit-in-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	mergedA, err := qtx.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "SAT-MERGED-A-" + suffix, Name: "Merged Reading Beginner Section 3 A " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	mergedB, err := qtx.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "SAT-MERGED-B-" + suffix, Name: "Merged Reading Beginner Section 3 B " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	standalone, err := qtx.CourseCreate(ctx, sqldb.CourseCreateParams{Code: "SAT-STANDALONE-" + suffix, Name: "Standalone Reading Beginner Section 1 " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	mergeGroup, err := qtx.CourseMergeGroupCreate(ctx, "SAT merged sit-in test "+suffix, teacherID)
	if err != nil {
		t.Fatal(err)
	}
	if err := qtx.CourseMergeGroupAssignCourse(ctx, mergeGroup.ID, mergedA.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := qtx.CourseMergeGroupAssignCourse(ctx, mergeGroup.ID, mergedB.ID, 2); err != nil {
		t.Fatal(err)
	}

	date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	createSession := func(courseID pgtype.UUID, hour int) pgtype.UUID {
		t.Helper()
		row, createErr := qtx.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:  courseID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: date.Add(time.Duration(hour) * time.Hour), Valid: true},
			EndAt:     pgtype.Timestamptz{Time: date.Add(time.Duration(hour)*time.Hour + time.Hour), Valid: true},
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return row.ID
	}
	missedSessionID := createSession(mergedA.ID, 9)
	mergedBSessionID := createSession(mergedB.ID, 10)
	createSession(standalone.ID, 11)

	mergedRule := satverbalpolicy.CourseRule{
		ID:         "sat-verbal-merged-section-3",
		CourseName: "SAT Verbal Reading Beginner Section 3",
		RuleType:   RuleTypeCrossSection,
		Priorities: []satverbalpolicy.RulePriority{{
			Level:         1,
			RuleType:      RuleTypeCrossSection,
			Label:         "Same Reading Beginner lesson in Section 1",
			MakeupTargets: []satverbalpolicy.Target{{Section: "Section 1", Subject: "Reading Beginner"}},
		}},
	}
	standaloneRule := satverbalpolicy.CourseRule{
		ID:         "sat-verbal-standalone-section-1",
		CourseName: "SAT Verbal Reading Beginner Section 1",
		RuleType:   RuleTypeCrossSection,
	}
	mergedRuleRaw, err := json.Marshal(mergedRule)
	if err != nil {
		t.Fatal(err)
	}
	standaloneRuleRaw, err := json.Marshal(standaloneRule)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qtx.SatVerbalPolicyMappingsReplace(ctx, []sqldb.SatVerbalPolicyMappingReplaceParam{
		{RuleID: mergedRule.ID, MergeGroupID: mergeGroup.ID, PolicyRule: mergedRuleRaw, PolicyHash: satverbalpolicy.HashPolicy(mergedRuleRaw)},
		{RuleID: standaloneRule.ID, CourseID: standalone.ID, PolicyRule: standaloneRuleRaw, PolicyHash: satverbalpolicy.HashPolicy(standaloneRuleRaw)},
	}); err != nil {
		t.Fatal(err)
	}
	standaloneID, err := uuidString(standalone.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertNonMergedTarget := func(missedCourseID, missedSessionID pgtype.UUID) {
		t.Helper()
		missedID, uuidErr := uuidString(missedSessionID)
		if uuidErr != nil {
			t.Fatal(uuidErr)
		}
		result, resolveErr := resolveMappedSatVerbalSitIn(ctx, qtx, pgtype.UUID{}, missedCourseID, nil, date, date, "Asia/Bangkok", 0)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if result == nil || len(result.Priorities) != 1 {
			t.Fatalf("merged missed course sit-in result = %#v, want one priority", result)
		}
		priority := result.Priorities[0]
		if priority.SitInCourse == nil || priority.SitInCourse.ID != standaloneID {
			t.Fatalf("sit-in target = %#v, want standalone non-merged course %s", priority.SitInCourse, standalone.ID)
		}
		if len(priority.Available) != 1 || priority.Available[0].CourseID != standaloneID {
			t.Fatalf("available sit-in sessions = %#v, want one standalone session", priority.Available)
		}
		if priority.Available[0].ID == missedID {
			t.Fatalf("sit-in session reused missed merged-course session %s", missedSessionID)
		}
	}
	assertNonMergedTarget(mergedA.ID, missedSessionID)
	assertNonMergedTarget(mergedB.ID, mergedBSessionID)
}
