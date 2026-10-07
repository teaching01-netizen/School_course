package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type courseLinkSuggestionPairFixture struct {
	source  pgtype.UUID
	partial pgtype.UUID
}

func TestCourseLinkSuggestions_DatabaseDecisionCases(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	q := New(dbpool).WithTx(tx)

	cycleID := createCourseLinkCycle(t, ctx, q, "2026 A", "2026-01-01", "2026-03-31")
	otherCycleID := createCourseLinkCycle(t, ctx, q, "2026 B", "2026-01-01", "2026-03-31")
	rootID := createCourseLinkRoot(t, ctx, q, "Root")
	otherRootID := createCourseLinkRoot(t, ctx, q, "Other Root")
	roomID, teacherID, otherTeacherID := createCourseLinkSessionActors(t, ctx, q)

	baseSourceDates := []time.Time{
		courseLinkUTCDate(2026, time.January, 4, 17, 0),
		courseLinkUTCDate(2026, time.January, 18, 17, 0),
	}
	partialDate := []time.Time{courseLinkUTCDate(2026, time.January, 11, 17, 0)}

	// Punctuation/case normalization, reliable period evidence, complementary
	// institute-local dates, and different teachers produce high confidence.
	high := createCourseLinkPair(t, ctx, q, rootID, "high", "English 101!", "english-101", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, high.source, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, high.partial, roomID, otherTeacherID, partialDate, time.Hour)
	assignCourseLinkTeacher(t, ctx, q, high.source, teacherID)
	assignCourseLinkTeacher(t, ctx, q, high.partial, otherTeacherID)

	// Thai letters and digits must survive the database normalization too.
	thai := createCourseLinkPair(t, ctx, q, rootID, "thai", "วิทย์ 2!", "วิทย์-2", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, thai.source, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, thai.partial, roomID, otherTeacherID, partialDate, time.Hour)

	unknownPeriod := createCourseLinkPair(t, ctx, q, rootID, "unknown-period", "Biology 1", "Biology-1", cycleID, nil, "", pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, unknownPeriod.source, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, unknownPeriod.partial, roomID, otherTeacherID, partialDate, time.Hour)

	slotMismatch := createCourseLinkPair(t, ctx, q, rootID, "slot-mismatch", "Chemistry 1", "Chemistry-1", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, slotMismatch.source, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, slotMismatch.partial, roomID, otherTeacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 11, 18, 0)}, time.Hour)

	durationMismatch := createCourseLinkPair(t, ctx, q, rootID, "duration-mismatch", "Physics 1", "Physics-1", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, durationMismatch.source, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, durationMismatch.partial, roomID, otherTeacherID, partialDate, 90*time.Minute)

	sameDate := createCourseLinkPair(t, ctx, q, rootID, "same-date", "History 1", "History-1", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, sameDate.source, roomID, teacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 4, 17, 0)}, time.Hour)
	seedCourseLinkSessions(t, ctx, q, sameDate.partial, roomID, otherTeacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 5, 16, 30)}, time.Hour)

	// Sessions that meet exactly at an end boundary are half-open and do not overlap.
	adjacent := createCourseLinkPair(t, ctx, q, rootID, "adjacent", "Language 1", "Language-1", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, adjacent.source, roomID, teacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 4, 17, 0)}, time.Hour)
	seedCourseLinkSessions(t, ctx, q, adjacent.partial, roomID, otherTeacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 4, 18, 0)}, time.Hour)

	overlap := createCourseLinkPair(t, ctx, q, rootID, "overlap", "Art 1", "Art-1", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, overlap.source, roomID, teacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 4, 17, 0)}, time.Hour)
	seedCourseLinkSessions(t, ctx, q, overlap.partial, roomID, otherTeacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 4, 17, 30)}, time.Hour)

	// A review candidate counts toward ambiguity before confidence filtering.
	ambiguousSubject := createCourseLinkSubject(t, ctx, q, "ambiguous")
	ambiguousSource := createCourseLinkCourse(t, ctx, q, ambiguousSubject, rootID, "ambiguous-source", "Geography 1", int16Ptr(2), cycleID)
	ambiguousHigh := createCourseLinkCourse(t, ctx, q, ambiguousSubject, pgtype.UUID{}, "ambiguous-high", "Geography-1", nil, cycleID)
	ambiguousReview := createCourseLinkCourse(t, ctx, q, ambiguousSubject, pgtype.UUID{}, "ambiguous-review", "Geography 1", nil, "")
	seedCourseLinkSessions(t, ctx, q, ambiguousSource, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, ambiguousHigh, roomID, otherTeacherID, partialDate, time.Hour)
	seedCourseLinkSessions(t, ctx, q, ambiguousReview, roomID, otherTeacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 25, 17, 0)}, time.Hour)
	ambiguous := courseLinkSuggestionPairFixture{source: ambiguousSource, partial: ambiguousHigh}
	ambiguousSecond := courseLinkSuggestionPairFixture{source: ambiguousSource, partial: ambiguousReview}

	// Also cover ambiguity from the other side: two configured sources share
	// one unconfigured course, so that course must not appear uniquely matched.
	sharedPartialSubject := createCourseLinkSubject(t, ctx, q, "shared-partial")
	sharedSourceA := createCourseLinkCourse(t, ctx, q, sharedPartialSubject, rootID, "shared-source-a", "Civics 1", int16Ptr(2), cycleID)
	sharedSourceB := createCourseLinkCourse(t, ctx, q, sharedPartialSubject, rootID, "shared-source-b", "Civics-1", int16Ptr(3), cycleID)
	sharedPartial := createCourseLinkCourse(t, ctx, q, sharedPartialSubject, pgtype.UUID{}, "shared-partial", "Civics 1", nil, "")
	seedCourseLinkSessions(t, ctx, q, sharedSourceA, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, sharedSourceB, roomID, otherTeacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, sharedPartial, roomID, teacherID, partialDate, time.Hour)
	sharedPartialA := courseLinkSuggestionPairFixture{source: sharedSourceA, partial: sharedPartial}
	sharedPartialB := courseLinkSuggestionPairFixture{source: sharedSourceB, partial: sharedPartial}

	// Hard exclusions: different subject/configuration, existing grouping,
	// SAT policy, dismissal, outside-cycle sessions, archived/deleted courses,
	// empty normalized names, and no live partial sessions.
	differentSubject := createCourseLinkPairDifferentSubjects(t, ctx, q, rootID, "different-subject", "Same Name 1", "same-name-1", cycleID)
	seedCourseLinkSessions(t, ctx, q, differentSubject.source, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, differentSubject.partial, roomID, otherTeacherID, partialDate, time.Hour)

	// Different Thai names sharing a digit must not collapse to the same key.
	differentThaiName := createCourseLinkPair(t, ctx, q, rootID, "different-thai-name", "วิทย์ 2", "ฟิสิกส์ 2", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, differentThaiName, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)

	differentName := createCourseLinkPair(t, ctx, q, rootID, "different-name", "Sociology 1", "Sociology 2", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, differentName, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)

	mismatchedLevel := createCourseLinkPair(t, ctx, q, rootID, "mismatched-level", "Algebra 1", "Algebra-1", cycleID, int16Ptr(1), "", pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, mismatchedLevel.source, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, mismatchedLevel.partial, roomID, otherTeacherID, partialDate, time.Hour)

	mismatchedCycle := createCourseLinkPair(t, ctx, q, rootID, "mismatched-cycle", "Geometry 1", "Geometry-1", cycleID, nil, otherCycleID, pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, mismatchedCycle, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)

	mismatchedRoot := createCourseLinkPair(t, ctx, q, rootID, "mismatched-root", "Economics 1", "Economics-1", cycleID, nil, "", otherRootID)
	seedCourseLinkSuggestionPairSessions(t, ctx, q, mismatchedRoot, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)

	grouped := createCourseLinkPair(t, ctx, q, rootID, "grouped", "Music 1", "Music-1", cycleID, nil, "", pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, grouped, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)
	groupID := insertCourseLinkGroup(t, ctx, q, "grouped")
	if _, err := q.db.Exec(ctx, `INSERT INTO course_merge_group_members (group_id, course_id, position) VALUES ($1, $2, 1)`, groupID, grouped.partial); err != nil {
		t.Fatal(err)
	}

	satPolicy := createCourseLinkPair(t, ctx, q, rootID, "sat-policy", "Verbal 1", "Verbal-1", cycleID, nil, "", pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, satPolicy, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)
	if _, err := q.db.Exec(ctx, `INSERT INTO sat_verbal_policy_mappings (rule_id, course_id, policy_rule, policy_hash) VALUES ($1, $2, '{}'::jsonb, 'test')`, "suggestion-"+uuid.NewString(), satPolicy.partial); err != nil {
		t.Fatal(err)
	}

	dismissed := createCourseLinkPair(t, ctx, q, rootID, "dismissed", "French 1", "French-1", cycleID, nil, "", pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, dismissed, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)
	actorID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "link-dismiss-" + uuid.NewString(), Role: "Admin", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if inserted, err := q.CourseLinkDismissalInsert(ctx, dismissed.source, dismissed.partial, actorID); err != nil || !inserted {
		t.Fatalf("insert dismissal: inserted=%v err=%v", inserted, err)
	}

	outsideCycle := createCourseLinkPair(t, ctx, q, rootID, "outside-cycle", "Latin 1", "Latin-1", cycleID, nil, cycleID, pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, outsideCycle.source, roomID, teacherID,
		[]time.Time{courseLinkUTCDate(2026, time.April, 5, 17, 0)}, time.Hour)
	seedCourseLinkSessions(t, ctx, q, outsideCycle.partial, roomID, otherTeacherID,
		[]time.Time{courseLinkUTCDate(2026, time.April, 12, 17, 0)}, time.Hour)

	emptyName := createCourseLinkPair(t, ctx, q, rootID, "empty-name", "---", "---", cycleID, nil, "", pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, emptyName, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)

	archived := createCourseLinkPair(t, ctx, q, rootID, "archived", "Drama 1", "Drama-1", cycleID, nil, "", pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, archived, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)
	if _, err := q.db.Exec(ctx, `UPDATE courses SET legacy_archived = true WHERE id = $1`, archived.source); err != nil {
		t.Fatal(err)
	}

	deleted := createCourseLinkPair(t, ctx, q, rootID, "deleted", "Politics 1", "Politics-1", cycleID, nil, "", pgtype.UUID{})
	seedCourseLinkSuggestionPairSessions(t, ctx, q, deleted, roomID, teacherID, otherTeacherID, baseSourceDates, partialDate)
	if _, err := q.db.Exec(ctx, `DELETE FROM courses WHERE id = $1`, deleted.partial); err != nil {
		t.Fatal(err)
	}

	noLiveSessions := createCourseLinkPair(t, ctx, q, rootID, "no-live-sessions", "Logic 1", "Logic-1", cycleID, nil, "", pgtype.UUID{})
	seedCourseLinkSessions(t, ctx, q, noLiveSessions.source, roomID, teacherID, baseSourceDates, time.Hour)
	seedCourseLinkSessionsDeleted(t, ctx, q, noLiveSessions.partial, roomID, otherTeacherID,
		courseLinkUTCDate(2026, time.January, 11, 17, 0), time.Hour)

	positivePairs := []courseLinkSuggestionPairFixture{high, thai, unknownPeriod, slotMismatch, durationMismatch, sameDate, adjacent, overlap, ambiguous, ambiguousSecond, sharedPartialA, sharedPartialB}
	excludedPairs := []courseLinkSuggestionPairFixture{differentSubject, differentThaiName, differentName, mismatchedLevel, mismatchedCycle, mismatchedRoot, grouped, satPolicy, dismissed, outsideCycle, emptyName, archived, deleted, noLiveSessions}
	rows := make([]CourseLinkSuggestionRow, 0, len(positivePairs))
	for _, pair := range positivePairs {
		pairRows := courseLinkSuggestionsForPair(t, ctx, q, pair, true)
		if len(pairRows) != 1 {
			t.Errorf("eligible pair %s returned %d rows", courseLinkPairKey(pair.source, pair.partial), len(pairRows))
			continue
		}
		rows = append(rows, pairRows[0])
	}
	for _, pair := range excludedPairs {
		if pairRows := courseLinkSuggestionsForPair(t, ctx, q, pair, true); len(pairRows) != 0 {
			t.Errorf("excluded pair %s returned rows: %v", courseLinkPairKey(pair.source, pair.partial), courseLinkSuggestionSummary(pairRows))
		}
	}

	highRow := requireCourseLinkSuggestion(t, rows, high)
	if highRow.Confidence != "high" {
		t.Fatalf("different teachers or UTC/local date boundary downgraded a valid pair: confidence=%s reasons=%v", highRow.Confidence, highRow.ReasonCodes)
	}
	if !containsCourseLinkReason(highRow.ReasonCodes, "same_teaching_period") || !containsCourseLinkReason(highRow.ReasonCodes, "complementary_dates") {
		t.Fatalf("high confidence evidence is incomplete: %v", highRow.ReasonCodes)
	}
	if len(highRow.SourceTeacherNames) != 1 || len(highRow.PartialTeacherNames) != 1 || highRow.SourceTeacherNames[0] == highRow.PartialTeacherNames[0] {
		t.Fatalf("teacher context should show different teachers without affecting matching: source=%v partial=%v", highRow.SourceTeacherNames, highRow.PartialTeacherNames)
	}
	if !highRow.SourceDateFrom.Valid || highRow.SourceDateFrom.String != "2026-01-05" || !highRow.PartialDateFrom.Valid || highRow.PartialDateFrom.String != "2026-01-12" || len(highRow.SourceSlots) != 1 || highRow.SourceSlots[0] != "1|00:00|3600" {
		t.Errorf("institute timezone was not applied to dates and slots: source=%v/%v partial=%v", highRow.SourceDateFrom, highRow.SourceSlots, highRow.PartialDateFrom)
	}
	if !containsCourseLinkReason(requireCourseLinkSuggestion(t, rows, unknownPeriod).ReasonCodes, "period_unknown") {
		t.Error("unknown cycle period was not reported as review evidence")
	}
	if !containsCourseLinkReason(requireCourseLinkSuggestion(t, rows, slotMismatch).ReasonCodes, "slot_mismatch") {
		t.Error("slot mismatch was not reported")
	}
	if !containsCourseLinkReason(requireCourseLinkSuggestion(t, rows, durationMismatch).ReasonCodes, "duration_mismatch") {
		t.Error("duration mismatch was not reported")
	}
	sameDateRow := requireCourseLinkSuggestion(t, rows, sameDate)
	if !containsCourseLinkReason(sameDateRow.ReasonCodes, "same_date_sessions") || sameDateRow.Confidence != "review" {
		t.Errorf("same-date sessions must remain review: confidence=%s reasons=%v", sameDateRow.Confidence, sameDateRow.ReasonCodes)
	}
	if containsCourseLinkReason(sameDateRow.ReasonCodes, "overlapping_sessions") {
		t.Errorf("different UTC dates on one institute-local date were incorrectly treated as overlapping: %v", sameDateRow.ReasonCodes)
	}
	adjacentRow := requireCourseLinkSuggestion(t, rows, adjacent)
	if !containsCourseLinkReason(adjacentRow.ReasonCodes, "same_date_sessions") || containsCourseLinkReason(adjacentRow.ReasonCodes, "overlapping_sessions") {
		t.Errorf("half-open adjacent intervals should share no overlap: %v", adjacentRow.ReasonCodes)
	}
	overlapRow := requireCourseLinkSuggestion(t, rows, overlap)
	if !containsCourseLinkReason(overlapRow.ReasonCodes, "overlapping_sessions") || overlapRow.Confidence != "review" {
		t.Errorf("overlapping sessions must remain review: confidence=%s reasons=%v", overlapRow.Confidence, overlapRow.ReasonCodes)
	}
	for _, pair := range []courseLinkSuggestionPairFixture{ambiguous, ambiguousSecond} {
		row := requireCourseLinkSuggestion(t, rows, pair)
		if row.Confidence != "review" || row.AmbiguityCount != 2 || !containsCourseLinkReason(row.ReasonCodes, "multiple_possible_partners") {
			t.Errorf("review partner must count toward ambiguity before filtering: confidence=%s count=%d reasons=%v", row.Confidence, row.AmbiguityCount, row.ReasonCodes)
		}
	}
	for _, pair := range []courseLinkSuggestionPairFixture{sharedPartialA, sharedPartialB} {
		row := requireCourseLinkSuggestion(t, rows, pair)
		if row.Confidence != "review" || row.AmbiguityCount != 2 || !containsCourseLinkReason(row.ReasonCodes, "multiple_possible_partners") {
			t.Errorf("multiple configured sources must make the partial ambiguous: confidence=%s count=%d reasons=%v", row.Confidence, row.AmbiguityCount, row.ReasonCodes)
		}
	}

	for _, pair := range []courseLinkSuggestionPairFixture{high, thai} {
		highOnly := courseLinkSuggestionsForPair(t, ctx, q, pair, false)
		if len(highOnly) != 1 || highOnly[0].Confidence != "high" {
			t.Errorf("high-only filter removed a valid high-confidence pair %s", courseLinkPairKey(pair.source, pair.partial))
		}
	}
	for _, pair := range []courseLinkSuggestionPairFixture{unknownPeriod, slotMismatch, durationMismatch, sameDate, adjacent, overlap, ambiguous, ambiguousSecond, sharedPartialA, sharedPartialB} {
		if highOnly := courseLinkSuggestionsForPair(t, ctx, q, pair, false); len(highOnly) != 0 {
			t.Errorf("high-only filter exposed review pair %s", courseLinkPairKey(pair.source, pair.partial))
		}
	}

	// Teacher-only edits update context but leave the decision fingerprint stable.
	fingerprint := highRow.EvidenceFingerprint
	if _, err := q.db.Exec(ctx, `DELETE FROM course_teachers WHERE course_id = $1`, high.source); err != nil {
		t.Fatal(err)
	}
	newTeacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "link-new-teacher-" + uuid.NewString(), Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.db.Exec(ctx, `UPDATE users SET full_name = 'Replacement Teacher' WHERE id = $1`, newTeacherID); err != nil {
		t.Fatal(err)
	}
	assignCourseLinkTeacher(t, ctx, q, high.source, newTeacherID)
	updatedTeacherRow := requireCourseLinkSuggestion(t, courseLinkSuggestionsForPair(t, ctx, q, high, true), high)
	if updatedTeacherRow.EvidenceFingerprint != fingerprint {
		t.Error("teacher-only change modified the decision fingerprint")
	}
	if len(updatedTeacherRow.SourceTeacherNames) != 1 || updatedTeacherRow.SourceTeacherNames[0] != "Replacement Teacher" {
		t.Errorf("updated teacher context not returned: %v", updatedTeacherRow.SourceTeacherNames)
	}

	// A schedule edit changes both confidence and the fingerprint.
	if _, err := q.db.Exec(ctx, `UPDATE sessions SET start_at = start_at + interval '1 hour', end_at = end_at + interval '1 hour' WHERE course_id = $1 AND deleted_at IS NULL`, high.partial); err != nil {
		t.Fatal(err)
	}
	changedRow := requireCourseLinkSuggestion(t, courseLinkSuggestionsForPair(t, ctx, q, high, true), high)
	if changedRow.EvidenceFingerprint == fingerprint || changedRow.Confidence != "review" {
		t.Errorf("schedule edit did not invalidate evidence: confidence=%s fingerprint_equal=%v", changedRow.Confidence, changedRow.EvidenceFingerprint == fingerprint)
	}
}

func TestCourseLinkSuggestions_CursorOrderIsStable(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := dbpool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	q := New(dbpool).WithTx(tx)
	cycleID := createCourseLinkCycle(t, ctx, q, "cursor", "2026-01-01", "2026-03-31")
	rootID := createCourseLinkRoot(t, ctx, q, "Cursor Root")
	roomID, teacherID, otherTeacherID := createCourseLinkSessionActors(t, ctx, q)
	subjectID := createCourseLinkSubject(t, ctx, q, "cursor")
	sourceID := createCourseLinkCourse(t, ctx, q, subjectID, rootID, "cursor-source", "Shared Course", int16Ptr(2), cycleID)
	seedCourseLinkSessions(t, ctx, q, sourceID, roomID, teacherID,
		[]time.Time{courseLinkUTCDate(2026, time.January, 4, 17, 0), courseLinkUTCDate(2026, time.January, 18, 17, 0)}, time.Hour)
	partialDates := []time.Time{
		courseLinkUTCDate(2026, time.January, 11, 17, 0),
		courseLinkUTCDate(2026, time.January, 25, 17, 0),
		courseLinkUTCDate(2026, time.February, 1, 17, 0),
	}
	for index, date := range partialDates {
		label := []string{"a", "b", "c"}[index]
		partialID := createCourseLinkCourse(t, ctx, q, subjectID, pgtype.UUID{}, "cursor-partial-"+label, "shared-course", nil, cycleID)
		seedCourseLinkSessions(t, ctx, q, partialID, roomID, otherTeacherID, []time.Time{date}, time.Hour)
	}

	first, err := q.CourseLinkSuggestions(ctx, CourseLinkSuggestionQuery{InstituteTZ: "Asia/Bangkok", IncludeReview: true, SourceCourseID: sourceID, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("first page has %d rows, want 1", len(first))
	}
	if first[0].Confidence != "review" || first[0].AmbiguityCount != 3 {
		t.Fatalf("pagination filtered ambiguity too early: confidence=%s ambiguity=%d", first[0].Confidence, first[0].AmbiguityCount)
	}
	second, err := q.CourseLinkSuggestions(ctx, CourseLinkSuggestionQuery{
		InstituteTZ: "Asia/Bangkok", IncludeReview: true, SourceCourseID: sourceID,
		Limit: 1, AfterSourceID: first[0].SourceID, AfterPartialID: first[0].PartialID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].AmbiguityCount != 3 {
		t.Fatalf("second page has %d rows, want 1", len(second))
	}
	if courseLinkPairKey(first[0].SourceID, first[0].PartialID) == courseLinkPairKey(second[0].SourceID, second[0].PartialID) {
		t.Fatal("cursor repeated the first page row")
	}

	searched, err := q.CourseLinkSuggestions(ctx, CourseLinkSuggestionQuery{
		InstituteTZ: "Asia/Bangkok", IncludeReview: true, SourceCourseID: sourceID,
		Search: strings.ToLower(second[0].PartialCode), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(searched) != 1 || courseLinkPairKey(searched[0].SourceID, searched[0].PartialID) != courseLinkPairKey(second[0].SourceID, second[0].PartialID) {
		t.Fatalf("search by partial code returned %d rows, want only that pair", len(searched))
	}
	if searched[0].AmbiguityCount != 3 {
		t.Fatalf("search narrowed ambiguity: got %d, want 3", searched[0].AmbiguityCount)
	}
	for _, search := range []string{"%", "_"} {
		rows, err := q.CourseLinkSuggestions(ctx, CourseLinkSuggestionQuery{
			InstituteTZ: "Asia/Bangkok", IncludeReview: true, SourceCourseID: sourceID, Search: search, Limit: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 0 {
			t.Fatalf("search %q matched %d rows as a wildcard", search, len(rows))
		}
	}
}

func createCourseLinkCycle(t *testing.T, ctx context.Context, q *Queries, label, startDate, endDate string) string {
	t.Helper()
	id := "course-link-" + uuid.NewString()
	if _, err := q.db.Exec(ctx, `INSERT INTO crm_cycles (id, label, source_kind, start_date, end_date) VALUES ($1, $2, 'manual', $3::date, $4::date)`, id, label+" "+id[len(id)-8:], startDate, endDate); err != nil {
		t.Fatal(err)
	}
	return id
}

func createCourseLinkRoot(t *testing.T, ctx context.Context, q *Queries, label string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := q.db.QueryRow(ctx, `INSERT INTO root_course_groups (name) VALUES ($1) RETURNING id`, label+" "+uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func createCourseLinkSubject(t *testing.T, ctx context.Context, q *Queries, label string) pgtype.UUID {
	t.Helper()
	suffix := uuid.NewString()
	subject, err := q.SubjectCreate(ctx, SubjectCreateParams{Code: "CLS-" + suffix, Name: label + " " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	return subject.ID
}

func createCourseLinkPair(t *testing.T, ctx context.Context, q *Queries, sourceRoot pgtype.UUID, label, sourceName, partialName, sourceCycle string, partialLevel *int16, partialCycle string, partialRoot pgtype.UUID) courseLinkSuggestionPairFixture {
	t.Helper()
	subjectID := createCourseLinkSubject(t, ctx, q, label)
	source := createCourseLinkCourse(t, ctx, q, subjectID, sourceRoot, label+"-source", sourceName, int16Ptr(2), sourceCycle)
	partial := createCourseLinkCourse(t, ctx, q, subjectID, partialRoot, label+"-partial", partialName, partialLevel, partialCycle)
	return courseLinkSuggestionPairFixture{source: source, partial: partial}
}

func createCourseLinkPairDifferentSubjects(t *testing.T, ctx context.Context, q *Queries, sourceRoot pgtype.UUID, label, sourceName, partialName, cycleID string) courseLinkSuggestionPairFixture {
	t.Helper()
	sourceSubject := createCourseLinkSubject(t, ctx, q, label+" source")
	partialSubject := createCourseLinkSubject(t, ctx, q, label+" partial")
	source := createCourseLinkCourse(t, ctx, q, sourceSubject, sourceRoot, label+"-source", sourceName, int16Ptr(2), cycleID)
	partial := createCourseLinkCourse(t, ctx, q, partialSubject, pgtype.UUID{}, label+"-partial", partialName, nil, "")
	return courseLinkSuggestionPairFixture{source: source, partial: partial}
}

func createCourseLinkCourse(t *testing.T, ctx context.Context, q *Queries, subjectID, rootID pgtype.UUID, label, name string, level *int16, cycleID string) pgtype.UUID {
	t.Helper()
	suffix := uuid.NewString()
	course, err := q.CourseCreate(ctx, CourseCreateParams{Code: "CLS-" + suffix, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	var levelValue any
	if level != nil {
		levelValue = *level
	}
	var rootValue any
	if rootID.Valid {
		rootValue = rootID
	}
	var cycleValue any
	if cycleID != "" {
		cycleValue = cycleID
	}
	if _, err := q.db.Exec(ctx, `UPDATE courses SET subject_id = $2, level = $3, cycle_id = $4, root_course_group_id = $5 WHERE id = $1`, course.ID, subjectID, levelValue, cycleValue, rootValue); err != nil {
		t.Fatalf("configure %s course %s: %v", label, course.Code, err)
	}
	return course.ID
}

func createCourseLinkSessionActors(t *testing.T, ctx context.Context, q *Queries) (pgtype.UUID, pgtype.UUID, pgtype.UUID) {
	t.Helper()
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "CLS-R-" + uuid.NewString(), Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	makeTeacher := func(name string) pgtype.UUID {
		id, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "cls-teacher-" + uuid.NewString(), Role: "Teacher", PasswordHash: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.db.Exec(ctx, `UPDATE users SET full_name = $1 WHERE id = $2`, name, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	return room.ID, makeTeacher("Teacher One"), makeTeacher("Teacher Two")
}

func assignCourseLinkTeacher(t *testing.T, ctx context.Context, q *Queries, courseID, teacherID pgtype.UUID) {
	t.Helper()
	if err := q.CourseTeacherInsert(ctx, CourseTeacherInsertParams{CourseID: courseID, TeacherID: teacherID, IsPrimary: true}); err != nil {
		t.Fatal(err)
	}
}

func seedCourseLinkSuggestionPairSessions(t *testing.T, ctx context.Context, q *Queries, pair courseLinkSuggestionPairFixture, roomID, teacherID, otherTeacherID pgtype.UUID, sourceDates, partialDates []time.Time) {
	t.Helper()
	seedCourseLinkSessions(t, ctx, q, pair.source, roomID, teacherID, sourceDates, time.Hour)
	seedCourseLinkSessions(t, ctx, q, pair.partial, roomID, otherTeacherID, partialDates, time.Hour)
}

func seedCourseLinkSessions(t *testing.T, ctx context.Context, q *Queries, courseID, _ pgtype.UUID, _ pgtype.UUID, starts []time.Time, duration time.Duration) {
	t.Helper()
	for _, start := range starts {
		roomID := createCourseLinkRoom(t, ctx, q)
		teacherID := createCourseLinkSessionTeacher(t, ctx, q)
		if _, err := q.db.Exec(ctx, `INSERT INTO sessions (course_id, room_id, teacher_id, start_at, end_at) VALUES ($1, $2, $3, $4, $5)`, courseID, roomID, teacherID, start, start.Add(duration)); err != nil {
			t.Fatal(err)
		}
	}
}

func seedCourseLinkSessionsDeleted(t *testing.T, ctx context.Context, q *Queries, courseID, _ pgtype.UUID, _ pgtype.UUID, start time.Time, duration time.Duration) {
	t.Helper()
	roomID := createCourseLinkRoom(t, ctx, q)
	teacherID := createCourseLinkSessionTeacher(t, ctx, q)
	if _, err := q.db.Exec(ctx, `INSERT INTO sessions (course_id, room_id, teacher_id, start_at, end_at, deleted_at) VALUES ($1, $2, $3, $4, $5, now())`, courseID, roomID, teacherID, start, start.Add(duration)); err != nil {
		t.Fatal(err)
	}
}

func createCourseLinkSessionTeacher(t *testing.T, ctx context.Context, q *Queries) pgtype.UUID {
	t.Helper()
	id, err := q.AdminUserCreate(ctx, AdminUserCreateParams{Username: "CLS-session-teacher-" + uuid.NewString(), Role: "Teacher", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func createCourseLinkRoom(t *testing.T, ctx context.Context, q *Queries) pgtype.UUID {
	t.Helper()
	room, err := q.RoomCreate(ctx, RoomCreateParams{Name: "CLS-session-room-" + uuid.NewString(), Capacity: pgtype.Int4{Int32: 20, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	return room.ID
}

func insertCourseLinkGroup(t *testing.T, ctx context.Context, q *Queries, label string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := q.db.QueryRow(ctx, `INSERT INTO course_merge_groups (name) VALUES ($1) RETURNING id`, label+" "+uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func courseLinkUTCDate(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

func int16Ptr(value int16) *int16 { return &value }

func courseLinkPairKey(sourceID, partialID pgtype.UUID) string {
	return sourceID.String() + "/" + partialID.String()
}

func courseLinkSuggestionKeys(rows []CourseLinkSuggestionRow) []string {
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, courseLinkPairKey(row.SourceID, row.PartialID))
	}
	return keys
}

func courseLinkSuggestionSummary(rows []CourseLinkSuggestionRow) []string {
	items := make([]string, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.SourceName+"/"+row.PartialName+"="+row.Confidence+"/"+strings.Join(row.ReasonCodes, ","))
	}
	return items
}

func requireCourseLinkSuggestion(t *testing.T, rows []CourseLinkSuggestionRow, pair courseLinkSuggestionPairFixture) CourseLinkSuggestionRow {
	t.Helper()
	key := courseLinkPairKey(pair.source, pair.partial)
	for _, row := range rows {
		if courseLinkPairKey(row.SourceID, row.PartialID) == key {
			return row
		}
	}
	t.Fatalf("suggestion %s not found in %v", key, courseLinkSuggestionKeys(rows))
	return CourseLinkSuggestionRow{}
}

func courseLinkSuggestionsForPair(t *testing.T, ctx context.Context, q *Queries, pair courseLinkSuggestionPairFixture, includeReview bool) []CourseLinkSuggestionRow {
	t.Helper()
	rows, err := q.CourseLinkSuggestions(ctx, CourseLinkSuggestionQuery{
		InstituteTZ: "Asia/Bangkok", IncludeReview: includeReview,
		SourceCourseID: pair.source, PartialCourseID: pair.partial, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func containsCourseLinkReason(reasons []string, wanted string) bool {
	for _, reason := range reasons {
		if reason == wanted {
			return true
		}
	}
	return false
}
