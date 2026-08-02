package serieshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func activeSeriesRows(t *testing.T, f seriesHTTPFixture, seriesID pgtype.UUID) []sqldb.SessionListActiveByCourseRow {
	t.Helper()
	rows, err := f.q.SessionListActiveByCourse(context.Background(), f.courseID)
	if err != nil {
		t.Fatal(err)
	}
	filtered := make([]sqldb.SessionListActiveByCourseRow, 0, len(rows))
	for _, row := range rows {
		if row.SeriesID.Valid && row.SeriesID.Bytes == seriesID.Bytes {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func TestUserStory_TenOccurrenceProposalRejectsConflictAtFifthOccurrenceWithoutWrites(t *testing.T) {
	// Given
	f := newSeriesHTTPFixture(t)
	ctx := context.Background()

	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().In(loc).AddDate(0, 0, 2)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	fifthOccurrence := start.AddDate(0, 0, 28)
	conflictStart := time.Date(fifthOccurrence.Year(), fifthOccurrence.Month(), fifthOccurrence.Day(), 10, 0, 0, 0, loc)
	conflictEnd := conflictStart.Add(time.Hour)

	conflictSession, err := f.q.SessionCreate(ctx, sqldb.SessionCreateParams{
		CourseID:  f.courseID,
		TeacherID: f.teacherID,
		StartAt:   pgtype.Timestamptz{Time: conflictStart.UTC(), Valid: true},
		EndAt:     pgtype.Timestamptz{Time: conflictEnd.UTC(), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	var beforeSeries, beforeSessions int
	if err := f.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM session_series WHERE course_id = $1),
			(SELECT count(*) FROM sessions WHERE course_id = $1)
	`, f.courseID).Scan(&beforeSeries, &beforeSessions); err != nil {
		t.Fatal(err)
	}
	if beforeSeries != 0 || beforeSessions != 1 {
		t.Fatalf("seeded rows=(series=%d,sessions=%d), want (0,1)", beforeSeries, beforeSessions)
	}

	// When
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"weekdays":[%d],"start_local_time":"10:00","duration_minutes":60,"start_date":%q,"count":10}`,
		pgUUIDString(t, f.courseID),
		pgUUIDString(t, f.teacherID),
		int(start.Weekday()),
		start.Format("2006-01-02"),
	))
	status, response := serveMutation(t, f.mux, http.MethodPost, "/api/v1/series", uuid.New().String(), body)

	// Then
	if status != http.StatusConflict {
		t.Fatalf("create series=(%d,%s), want 409", status, response)
	}

	var decoded struct {
		Code    string `json:"code"`
		Details struct {
			Kind      string `json:"kind"`
			Requested struct {
				StartAt string `json:"start_at"`
				EndAt   string `json:"end_at"`
			} `json:"requested"`
			Conflicts []struct {
				SessionID string `json:"session_id"`
				StartAt   string `json:"start_at"`
				EndAt     string `json:"end_at"`
			} `json:"conflicts"`
		} `json:"details"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Code != "schedule_conflict" {
		t.Fatalf("response code=%q, want schedule_conflict", decoded.Code)
	}
	if decoded.Details.Kind != "teacher_overlap" {
		t.Fatalf("conflict kind=%q, want teacher_overlap", decoded.Details.Kind)
	}
	wantStart := conflictStart.UTC().Format(time.RFC3339Nano)
	wantEnd := conflictEnd.UTC().Format(time.RFC3339Nano)
	if decoded.Details.Requested.StartAt != wantStart || decoded.Details.Requested.EndAt != wantEnd {
		t.Fatalf("requested=(%s,%s), want fifth occurrence=(%s,%s)", decoded.Details.Requested.StartAt, decoded.Details.Requested.EndAt, wantStart, wantEnd)
	}
	wantConflictID := pgUUIDString(t, conflictSession.ID)
	if len(decoded.Details.Conflicts) != 1 {
		t.Fatalf("conflicts=%+v, want seeded session %s", decoded.Details.Conflicts, wantConflictID)
	}
	conflict := decoded.Details.Conflicts[0]
	if conflict.SessionID != wantConflictID || conflict.StartAt != wantStart || conflict.EndAt != wantEnd {
		t.Fatalf("conflict=%+v, want seeded session %s at (%s,%s)", conflict, wantConflictID, wantStart, wantEnd)
	}

	var afterSeries, afterSessions int
	if err := f.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM session_series WHERE course_id = $1),
			(SELECT count(*) FROM sessions WHERE course_id = $1)
	`, f.courseID).Scan(&afterSeries, &afterSessions); err != nil {
		t.Fatal(err)
	}
	if afterSeries != beforeSeries || afterSessions != beforeSessions {
		t.Fatalf("rows changed after conflict: before=(series=%d,sessions=%d), after=(series=%d,sessions=%d)", beforeSeries, beforeSessions, afterSeries, afterSessions)
	}
}

func TestUserStory_CreateTenOccurrenceSeriesPersistsTenOccurrencesAndReplaysIdempotently(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().In(loc).AddDate(0, 0, 3)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"weekdays":[%d],"start_local_time":"10:00","duration_minutes":60,"start_date":%q,"count":10}`,
		pgUUIDString(t, f.courseID), pgUUIDString(t, f.teacherID), int(start.Weekday()), start.Format("2006-01-02")))
	key := "user-story-series-create-" + uuid.NewString()

	status1, response1 := serveMutation(t, f.mux, http.MethodPost, "/api/v1/series", key, body)
	if status1 != http.StatusCreated {
		t.Fatalf("create=(%d,%s), want 201", status1, response1)
	}
	var decoded struct {
		SeriesID      string `json:"series_id"`
		SessionsAdded int    `json:"sessions_added"`
	}
	if err := json.Unmarshal(response1, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SessionsAdded != 10 {
		t.Fatalf("sessions_added=%d, want 10", decoded.SessionsAdded)
	}
	seriesID := mustPgUUID(t, decoded.SeriesID)

	status2, response2 := serveMutation(t, f.mux, http.MethodPost, "/api/v1/series", key, body)
	if status2 != status1 || !bytes.Equal(response2, response1) {
		t.Fatalf("replay=(%d,%s), original=(%d,%s)", status2, response2, status1, response1)
	}

	var seriesCount int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM session_series WHERE course_id=$1 AND id=$2`, f.courseID, seriesID).Scan(&seriesCount); err != nil {
		t.Fatal(err)
	}
	if seriesCount != 1 {
		t.Fatalf("fixture series rows=%d, want 1", seriesCount)
	}
	rows := activeSeriesRows(t, f, seriesID)
	if len(rows) != 10 {
		t.Fatalf("active occurrences=%d, want 10", len(rows))
	}
	seen := make(map[[16]byte]struct{}, len(rows))
	for _, row := range rows {
		seen[row.ID.Bytes] = struct{}{}
	}
	if len(seen) != 10 {
		t.Fatalf("distinct active occurrences=%d, want 10", len(seen))
	}
}

func TestUserStory_ThisAndFuturePivotPreservesHistoryAndMovesFutureRows(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	seriesID, first := f.createCountSeries(t, 10)
	pivot := first.AddDate(0, 0, 28)
	body := []byte(fmt.Sprintf(`{"pivot_date":%q,"expected_version":1,"start_local_time":"12:00","count":10}`,
		pivot.Format("2006-01-02")))
	status, response := serveMutation(t, f.mux, http.MethodPatch, "/api/v1/series/"+seriesID, uuid.NewString(), body)
	if status != http.StatusOK {
		t.Fatalf("split=(%d,%s), want 200", status, response)
	}
	var decoded struct {
		OldSeriesID string `json:"old_series_id"`
		NewSeriesID string `json:"new_series_id"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OldSeriesID != seriesID || decoded.NewSeriesID == "" || decoded.NewSeriesID == seriesID {
		t.Fatalf("split ids=(%s,%s), want old=%s and distinct new id", decoded.OldSeriesID, decoded.NewSeriesID, seriesID)
	}
	oldID := mustPgUUID(t, decoded.OldSeriesID)
	newID := mustPgUUID(t, decoded.NewSeriesID)
	rows := activeSeriesRows(t, f, oldID)
	rows = append(rows, activeSeriesRows(t, f, newID)...)
	if len(rows) != 10 {
		t.Fatalf("active occurrences after split=%d, want 10", len(rows))
	}
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	oldCount, newCount := 0, 0
	for _, row := range rows {
		local := row.StartAt.Time.In(loc)
		switch {
		case row.SeriesID.Bytes == oldID.Bytes:
			oldCount++
			if !local.Before(pivot) || local.Hour() != 10 {
				t.Fatalf("retained occurrence=%s, want before pivot at 10:00", local)
			}
		case row.SeriesID.Bytes == newID.Bytes:
			newCount++
			if local.Before(pivot) || local.Hour() != 12 {
				t.Fatalf("future occurrence=%s, want on/after pivot at 12:00", local)
			}
		default:
			t.Fatalf("unexpected series id %x", row.SeriesID.Bytes)
		}
	}
	if oldCount != 4 || newCount != 6 {
		t.Fatalf("pivot partition=(old=%d,new=%d), want (4,6)", oldCount, newCount)
	}
}

func TestUserStory_EditEntireSeriesStaleVersionReplaysStableResponse(t *testing.T) {
	f := newSeriesHTTPFixture(t)
	seriesID, start := f.createCountSeries(t, 10)
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"weekdays":[%d],"start_local_time":"11:00","duration_minutes":90,"count":10,"expected_version":0}`,
		pgUUIDString(t, f.courseID), pgUUIDString(t, f.teacherID), int(start.Weekday())))
	key := "user-story-series-stale-" + uuid.NewString()
	path := "/api/v1/series/" + seriesID + "/entire"

	status1, response1 := serveMutation(t, f.mux, http.MethodPatch, path, key, body)
	if status1 != http.StatusConflict {
		t.Fatalf("stale edit=(%d,%s), want 409", status1, response1)
	}
	var decoded struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response1, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Code != "stale_edit" {
		t.Fatalf("stale code=%q, want stale_edit", decoded.Code)
	}
	status2, response2 := serveMutation(t, f.mux, http.MethodPatch, path, key, body)
	if status2 != status1 || !bytes.Equal(response2, response1) {
		t.Fatalf("stale replay=(%d,%s), original=(%d,%s)", status2, response2, status1, response1)
	}
	row, err := f.q.SeriesGetByID(context.Background(), mustPgUUID(t, seriesID))
	if err != nil {
		t.Fatal(err)
	}
	if row.Version != 1 || row.DurationMinutes != 60 || row.StartLocalTime.Microseconds != 10*60*60*1_000_000 {
		t.Fatalf("series changed after stale replay: version=%d duration=%d start=%d", row.Version, row.DurationMinutes, row.StartLocalTime.Microseconds)
	}
	if rows := activeSeriesRows(t, f, row.ID); len(rows) != 10 {
		t.Fatalf("active occurrences after stale replay=%d, want 10", len(rows))
	}
}
