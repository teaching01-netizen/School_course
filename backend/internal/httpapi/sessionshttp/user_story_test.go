package sessionshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/scheduling"
	"warwick-institute/internal/series"
)

func TestUserStory_AdminCreatesAvailableSession(t *testing.T) {
	f := newScheduleHTTPFixture(t)
	ctx := context.Background()

	room, err := f.q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "USER-STORY-ROOM-" + uuid.New().String()[:8],
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	actorPgID, err := f.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "user-story-admin-" + uuid.New().String()[:8],
		Role:         "Admin",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	actorID, err := uuid.FromBytes(actorPgID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}
	seriesSvc, err := series.NewService(f.pool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	schedulingSvc, err := scheduling.NewService(f.pool, "Asia/Bangkok", seriesSvc)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, httpdeps.Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth: fakeAuth{user: auth.AuthenticatedUser{
			ID: actorID, Username: "user-story-admin", Role: "Admin",
		}},
		Q:           f.q,
		DB:          f.pool,
		Scheduling:  schedulingSvc,
		InstituteTZ: "Asia/Bangkok",
	})

	start := futureLocalStart(t, 20, 10)
	end := start.Add(time.Hour)
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"room_id":%q,"start_at":%q,"end_at":%q}`,
		uuidString(t, f.courseID), uuidString(t, f.teacherID), uuidString(t, room.ID),
		start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)))
	key := uuid.New().String()

	status1, response1 := serveMutation(t, mux, http.MethodPost, "/api/v1/sessions", key, body)
	if status1 != http.StatusCreated {
		t.Fatalf("first response=(%d,%s), want 201", status1, response1)
	}
	var statusCode *int32
	var responseBody []byte
	if err := f.pool.QueryRow(ctx, `
		SELECT status_code, response_body
		FROM idempotency_keys
		WHERE actor_user_id = $1
		  AND scope = $2
		  AND idempotency_key = $3
	`, actorPgID, "sessions", key).Scan(&statusCode, &responseBody); err != nil {
		t.Fatalf("query completed idempotency record: %v", err)
	}
	if statusCode == nil || *statusCode != http.StatusCreated {
		t.Fatalf("idempotency status_code=%v, want %d", statusCode, http.StatusCreated)
	}
	if len(responseBody) == 0 {
		t.Fatal("idempotency response_body is empty, want completed response")
	}

	status2, response2 := serveMutation(t, mux, http.MethodPost, "/api/v1/sessions", key, body)
	if status2 != status1 || !bytes.Equal(response2, response1) {
		t.Fatalf("replay=(%d,%s), original=(%d,%s)", status2, response2, status1, response1)
	}

	items, err := f.q.SessionListActiveByCourse(ctx, f.courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("active sessions=%d, want 1", len(items))
	}
	item := items[0]
	if item.CourseID != f.courseID || item.TeacherID != f.teacherID || item.RoomID != room.ID {
		t.Fatalf("resources=(course=%s teacher=%s room=%s), want=(course=%s teacher=%s room=%s)",
			uuidString(t, item.CourseID), uuidString(t, item.TeacherID), uuidString(t, item.RoomID),
			uuidString(t, f.courseID), uuidString(t, f.teacherID), uuidString(t, room.ID))
	}
	if !item.StartAt.Time.Equal(start.UTC()) || !item.EndAt.Time.Equal(end.UTC()) {
		t.Fatalf("interval=(%s,%s), want=(%s,%s)", item.StartAt.Time, item.EndAt.Time, start.UTC(), end.UTC())
	}
	if item.Version != 1 {
		t.Fatalf("version=%d, want 1", item.Version)
	}
}

func TestUserStory_AdminCreatesProvisionalSessionWithoutRoom(t *testing.T) {
	f := newScheduleHTTPFixture(t)
	ctx := context.Background()
	start := futureLocalStart(t, 22, 10)
	end := start.Add(time.Hour)
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"room_id":null,"start_at":%q,"end_at":%q}`,
		uuidString(t, f.courseID), uuidString(t, f.teacherID),
		start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)))
	key := uuid.NewString()

	status1, response1 := serveMutation(t, f.mux, http.MethodPost, "/api/v1/sessions", key, body)
	if status1 != http.StatusCreated {
		t.Fatalf("first response=(%d,%s), want 201", status1, response1)
	}

	status2, response2 := serveMutation(t, f.mux, http.MethodPost, "/api/v1/sessions", key, body)
	if status2 != status1 || !bytes.Equal(response2, response1) {
		t.Fatalf("replay=(%d,%s), original=(%d,%s)", status2, response2, status1, response1)
	}

	items, err := f.q.SessionListActiveByCourse(ctx, f.courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("active sessions for fixture course=%d, want 1 after replay", len(items))
	}
	item := items[0]
	if item.CourseID != f.courseID || item.TeacherID != f.teacherID {
		t.Fatalf("resources=(course=%s teacher=%s), want=(course=%s teacher=%s)",
			uuidString(t, item.CourseID), uuidString(t, item.TeacherID),
			uuidString(t, f.courseID), uuidString(t, f.teacherID))
	}
	if item.RoomID.Valid {
		t.Fatalf("persisted room_id=%s, want NULL", uuidString(t, item.RoomID))
	}
	if !item.StartAt.Time.Equal(start.UTC()) || !item.EndAt.Time.Equal(end.UTC()) {
		t.Fatalf("interval=(%s,%s), want=(%s,%s)", item.StartAt.Time, item.EndAt.Time, start.UTC(), end.UTC())
	}

	unassignedTeacher, err := f.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "user-story-unassigned-teacher-" + uuid.NewString()[:8],
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	emptyCourse, err := f.q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "USER-STORY-EMPTY-" + uuid.NewString()[:8], Name: "User story empty teacher set",
	})
	if err != nil {
		t.Fatal(err)
	}
	validationCases := []struct {
		name      string
		courseID  string
		teacherID string
		start     time.Time
		end       time.Time
		status    int
		code      string
	}{
		{
			name:      "unassigned teacher",
			courseID:  uuidString(t, f.courseID),
			teacherID: uuidString(t, unassignedTeacher),
			start:     start.Add(2 * time.Hour),
			end:       start.Add(3 * time.Hour),
			status:    http.StatusConflict,
			code:      "teacher_not_assigned_to_course",
		},
		{
			name:      "course without assigned teachers",
			courseID:  uuidString(t, emptyCourse.ID),
			teacherID: uuidString(t, f.teacherID),
			start:     start.Add(4 * time.Hour),
			end:       start.Add(5 * time.Hour),
			status:    http.StatusConflict,
			code:      "course_has_no_assigned_teachers",
		},
		{
			name:      "invalid time range",
			courseID:  uuidString(t, f.courseID),
			teacherID: uuidString(t, f.teacherID),
			start:     start.Add(6 * time.Hour),
			end:       start.Add(5 * time.Hour),
			status:    http.StatusBadRequest,
			code:      "bad_range",
		},
	}
	for _, tc := range validationCases {
		t.Run(tc.name, func(t *testing.T) {
			validationBody := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"room_id":null,"start_at":%q,"end_at":%q}`,
				tc.courseID, tc.teacherID,
				tc.start.UTC().Format(time.RFC3339), tc.end.UTC().Format(time.RFC3339)))
			status, response := serveMutation(t, f.mux, http.MethodPost, "/api/v1/sessions", uuid.NewString(), validationBody)
			if status != tc.status || !bytes.Contains(response, []byte(fmt.Sprintf(`"code":%q`, tc.code))) {
				t.Fatalf("response=(%d,%s), want=(%d,%s)", status, response, tc.status, tc.code)
			}
		})
	}

	items, err = f.q.SessionListActiveByCourse(ctx, f.courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("active sessions for fixture course=%d, want 1 after validation rejections", len(items))
	}

	t.Run("nil room teacher overlap is a schedule conflict", func(t *testing.T) {
		overlapFixture := newScheduleHTTPFixture(t)
		overlapStart := futureLocalStart(t, 23, 10)
		overlapEnd := overlapStart.Add(time.Hour)
		existing, err := overlapFixture.q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:  overlapFixture.courseID,
			TeacherID: overlapFixture.teacherID,
			StartAt:   pgtype.Timestamptz{Time: overlapStart.UTC(), Valid: true},
			EndAt:     pgtype.Timestamptz{Time: overlapEnd.UTC(), Valid: true},
		})
		if err != nil {
			t.Fatal(err)
		}

		overlapBody := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"room_id":null,"start_at":%q,"end_at":%q}`,
			uuidString(t, overlapFixture.courseID), uuidString(t, overlapFixture.teacherID),
			overlapStart.UTC().Format(time.RFC3339), overlapEnd.UTC().Format(time.RFC3339)))
		status, response := serveMutation(t, overlapFixture.mux, http.MethodPost, "/api/v1/sessions", uuid.NewString(), overlapBody)
		var decoded struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response, &decoded); err != nil {
			t.Fatalf("decode overlap response %q: %v", response, err)
		}
		if status != http.StatusConflict || decoded.Code != "schedule_conflict" {
			t.Fatalf("response=(%d,%s), want 409 schedule_conflict", status, response)
		}

		overlapItems, err := overlapFixture.q.SessionListActiveByCourse(ctx, overlapFixture.courseID)
		if err != nil {
			t.Fatal(err)
		}
		if len(overlapItems) != 1 || overlapItems[0].ID != existing.ID {
			t.Fatalf("active sessions for fixture course=%d, want 1 existing session after conflict", len(overlapItems))
		}
	})
}

func TestUserStory_TwoAdminsSubmitSameSlot_OnlyOneSucceeds(t *testing.T) {
	f := newScheduleHTTPFixture(t)
	ctx := context.Background()

	room, err := f.q.RoomCreate(ctx, sqldb.RoomCreateParams{
		Name:     "USER-STORY-RACE-ROOM-" + uuid.New().String()[:8],
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	adminPgID, err := f.q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "user-story-admin-" + uuid.New().String()[:8],
		Role:         "Admin",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	adminID, err := uuid.FromBytes(adminPgID.Bytes[:])
	if err != nil {
		t.Fatal(err)
	}

	seriesSvc, err := series.NewService(f.pool, "Asia/Bangkok")
	if err != nil {
		t.Fatal(err)
	}
	schedulingSvc, err := scheduling.NewService(f.pool, "Asia/Bangkok", seriesSvc)
	if err != nil {
		t.Fatal(err)
	}
	secondMux := http.NewServeMux()
	Register(secondMux, httpdeps.Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth: fakeAuth{user: auth.AuthenticatedUser{
			ID: adminID, Username: "second-admin", Role: "Admin",
		}},
		Q:           f.q,
		DB:          f.pool,
		Scheduling:  schedulingSvc,
		InstituteTZ: "Asia/Bangkok",
	})

	start := futureLocalStart(t, 21, 10)
	end := start.Add(time.Hour)
	body := []byte(fmt.Sprintf(`{"course_id":%q,"teacher_id":%q,"room_id":%q,"start_at":%q,"end_at":%q}`,
		uuidString(t, f.courseID), uuidString(t, f.teacherID), uuidString(t, room.ID),
		start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)))

	type result struct {
		status   int
		response []byte
	}
	ready := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, handler := range []http.Handler{f.mux, secondMux} {
		wg.Add(1)
		go func(handler http.Handler) {
			defer wg.Done()
			<-ready
			status, response := serveMutation(t, handler, http.MethodPost, "/api/v1/sessions", uuid.New().String(), body)
			results <- result{status: status, response: response}
		}(handler)
	}
	close(ready)
	wg.Wait()
	close(results)

	created := 0
	conflicts := 0
	for response := range results {
		if response.status == http.StatusInternalServerError {
			t.Fatalf("unexpected 500 response=%s", response.response)
		}
		switch response.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(response.response, &body); err != nil {
				t.Fatalf("decode conflict response %q: %v", response.response, err)
			}
			if body.Code != "schedule_conflict" {
				t.Fatalf("conflict code=%q, want schedule_conflict; response=%s", body.Code, response.response)
			}
			conflicts++
		default:
			t.Fatalf("unexpected response status=%d body=%s", response.status, response.response)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("created=%d conflicts=%d, want one of each", created, conflicts)
	}

	items, err := f.q.SessionListActiveByCourse(ctx, f.courseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("active sessions=%d, want 1", len(items))
	}
	item := items[0]
	if item.CourseID != f.courseID || item.TeacherID != f.teacherID || item.RoomID != room.ID {
		t.Fatalf("resources=(course=%s teacher=%s room=%s), want=(course=%s teacher=%s room=%s)",
			uuidString(t, item.CourseID), uuidString(t, item.TeacherID), uuidString(t, item.RoomID),
			uuidString(t, f.courseID), uuidString(t, f.teacherID), uuidString(t, room.ID))
	}
	if !item.StartAt.Time.Equal(start.UTC()) || !item.EndAt.Time.Equal(end.UTC()) {
		t.Fatalf("interval=(%s,%s), want=(%s,%s)", item.StartAt.Time, item.EndAt.Time, start.UTC(), end.UTC())
	}
	if item.Version != 1 {
		t.Fatalf("version=%d, want 1", item.Version)
	}
}
