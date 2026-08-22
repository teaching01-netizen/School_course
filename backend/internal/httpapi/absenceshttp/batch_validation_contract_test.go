package absenceshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
	"warwick-institute/internal/studentauth"
)

type publicAbsenceContractFixture struct {
	pool               *pgxpool.Pool
	server             *server
	sessionToken       string
	wcode              string
	subjectID          string
	courseID           string
	missedSessionIDs   []string
	missedSessionDates []string
	unrelatedCourseID  string
	unrelatedSessionID string
}

func newPublicAbsenceContractFixture(t *testing.T) *publicAbsenceContractFixture {
	t.Helper()
	databaseURL := requireTestDBPending(t)
	migrateUpOncePending(t, databaseURL)
	pool := newPoolPending(t, databaseURL)
	t.Cleanup(pool.Close)

	q := sqldb.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	previousSettings, err := q.AppSettingsGetWithPolicies(ctx)
	if err != nil {
		t.Fatalf("load existing absence settings: %v", err)
	}
	previousPolicies := append([]byte(nil), previousSettings.AbsencePolicies...)
	t.Cleanup(func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer restoreCancel()
		if err := q.AppSettingsUpdateAbsencePolicies(restoreCtx, previousPolicies); err != nil {
			t.Errorf("restore absence settings: %v", err)
		}
	})

	settings := defaultAbsenceSettings()
	settings.Notifications.SmsParentEnabled = false
	settings.Notifications.SmsSuccessTemplate = ""
	settings.Notifications.EmailSuccessEnabled = false
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal test absence settings: %v", err)
	}
	if err := q.AppSettingsUpdateAbsencePolicies(ctx, settingsJSON); err != nil {
		t.Fatalf("install test absence settings: %v", err)
	}

	suffix := uuid.NewString()[:8]
	wcode, subjectID, courseID, missedSessionIDs := seedAbsenceLimitTestData(t, q, pool, "CTR-"+suffix, 10)
	missedSessionDates := make([]string, len(missedSessionIDs))
	for index := range missedSessionDates {
		missedSessionDates[index] = time.Date(2026, time.June, index+1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}

	unrelatedSubject, err := q.SubjectCreate(ctx, sqldb.SubjectCreateParams{
		Code: "UNRELATED-SUB-" + suffix,
		Name: "Unrelated subject " + suffix,
	})
	if err != nil {
		t.Fatalf("create unrelated subject: %v", err)
	}

	unrelatedCourse, err := q.CourseCreate(ctx, sqldb.CourseCreateParams{
		Code: "UNRELATED-COURSE-" + suffix,
		Name: "Unrelated course " + suffix,
	})
	if err != nil {
		t.Fatalf("create unrelated course: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE courses SET subject_id = $1 WHERE id = $2`, unrelatedSubject.ID, unrelatedCourse.ID); err != nil {
		t.Fatalf("link unrelated course to subject: %v", err)
	}

	teacherID, err := q.AdminUserCreate(ctx, sqldb.AdminUserCreateParams{
		Username:     "unrelated-teacher-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("create unrelated teacher: %v", err)
	}

	var unrelatedSessionID string
	for day := 1; day <= 2; day++ {
		start := time.Date(2030, time.February, day, 9, 0, 0, 0, time.UTC)
		session, err := q.SessionCreate(ctx, sqldb.SessionCreateParams{
			CourseID:  unrelatedCourse.ID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: start.Add(90 * time.Minute), Valid: true},
		})
		if err != nil {
			t.Fatalf("create unrelated session %d: %v", day, err)
		}
		if day == 1 {
			unrelatedSessionID, err = uuidString(session.ID)
			if err != nil {
				t.Fatalf("format unrelated session ID: %v", err)
			}
		}
	}

	unrelatedCourseID, err := uuidString(unrelatedCourse.ID)
	if err != nil {
		t.Fatalf("format unrelated course ID: %v", err)
	}

	// The batch endpoint authenticates via verified student session; identity
	// comes from the session cookie, never from the request body.
	sessionToken := seedVerifiedStudentSession(t, pool, wcode)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &server{
		deps: httpdeps.Deps{
			Q:           q,
			DB:          pool,
			Log:         logger,
			InstituteTZ: "UTC",
		},
		a: httpadapter.New(nil, logger),
	}

	return &publicAbsenceContractFixture{
		pool:               pool,
		server:             s,
		sessionToken:       sessionToken,
		wcode:              wcode,
		subjectID:          subjectID,
		courseID:           courseID,
		missedSessionIDs:   missedSessionIDs,
		missedSessionDates: missedSessionDates,
		unrelatedCourseID:  unrelatedCourseID,
		unrelatedSessionID: unrelatedSessionID,
	}
}

func contractStringPtr(value string) *string {
	return &value
}

func (f *publicAbsenceContractFixture) validItem(index int) batchAbsenceCreateItem {
	return batchAbsenceCreateItem{
		SubjectID:        f.subjectID,
		CourseID:         f.courseID,
		DateFrom:         f.missedSessionDates[index],
		DateTo:           f.missedSessionDates[index],
		MissedSessionIDs: []string{f.missedSessionIDs[index]},
	}
}

func (f *publicAbsenceContractFixture) requestBody(t *testing.T, items ...batchAbsenceCreateItem) []byte {
	t.Helper()
	body, err := json.Marshal(batchAbsenceCreateRequest{Items: items})
	if err != nil {
		t.Fatalf("marshal batch request: %v", err)
	}
	return body
}

func (f *publicAbsenceContractFixture) submitBatch(body []byte, idempotencyKey string) *httptest.ResponseRecorder {
	return f.submitBatchAs(body, idempotencyKey, f.sessionToken)
}

func (f *publicAbsenceContractFixture) submitBatchAs(body []byte, idempotencyKey, sessionToken string) *httptest.ResponseRecorder {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/batch", bytes.NewReader(body)).WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: sessionToken})
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	recorder := httptest.NewRecorder()
	f.server.handleAbsenceBatchCreate(recorder, req)
	return recorder
}

// reverify replaces the (burned) student session with a fresh verified one,
// mirroring the re-verify step a client performs after a submission revoked
// its session. Idempotency is keyed by student, not session, so held keys
// remain replayable after the rotation.
func (f *publicAbsenceContractFixture) reverify(t *testing.T) {
	t.Helper()
	f.sessionToken = seedVerifiedStudentSession(t, f.pool, f.wcode)
}

func (f *publicAbsenceContractFixture) absenceCount(t *testing.T) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM student_absences WHERE wcode = $1`, f.wcode).Scan(&count); err != nil {
		t.Fatalf("count absences: %v", err)
	}
	return count
}

type contractBatchResponse struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
}

func decodeContractBatchResponse(t *testing.T, recorder *httptest.ResponseRecorder) contractBatchResponse {
	t.Helper()
	var got contractBatchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode batch response %q: %v", recorder.Body.String(), err)
	}
	return got
}

func assertRejectedBatchContract(t *testing.T, fixture *publicAbsenceContractFixture, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; persisted absences = %d; body = %s", recorder.Code, fixture.absenceCount(t), recorder.Body.String())
	}
	got := decodePublicContractError(t, recorder)
	if got.Code == "" {
		t.Fatalf("rejected batch response has no error code: %s", recorder.Body.String())
	}
	if count := fixture.absenceCount(t); count != 0 {
		t.Fatalf("rejected batch persisted %d absences, want 0", count)
	}
}

func TestPublicBatchRejectsEmptyMissedSessionIDs(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)
	item := fixture.validItem(0)
	item.MissedSessionIDs = []string{}

	recorder := fixture.submitBatch(fixture.requestBody(t, item), uuid.NewString())

	assertRejectedBatchContract(t, fixture, recorder)
}

func TestPublicBatchAllowsSitInCourseOutsideSelectedSubject(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)
	item := fixture.validItem(0)
	item.SitInMethod = contractStringPtr(SitInMethodPhysical)
	item.SitInCourseID = contractStringPtr(fixture.unrelatedCourseID)
	item.SitInSessionIDs = []string{fixture.unrelatedSessionID}

	recorder := fixture.submitBatch(fixture.requestBody(t, item), uuid.NewString())

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", recorder.Code, recorder.Body.String())
	}
	got := decodeContractBatchResponse(t, recorder)
	if len(got.Items) != 1 {
		t.Fatalf("response items = %d, want 1; body = %s", len(got.Items), recorder.Body.String())
	}
	if got.Items[0].ID == "" {
		t.Fatalf("created item has no ID: %s", recorder.Body.String())
	}
	if count := fixture.absenceCount(t); count != 1 {
		t.Fatalf("persisted absences = %d, want 1", count)
	}
}

func TestPublicBatchRollsBackEveryItemWhenLaterItemFails(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)
	first := fixture.validItem(0)
	second := fixture.validItem(1)
	second.MissedSessionIDs = []string{"not-a-uuid"}
	key := uuid.NewString()

	recorder := fixture.submitBatch(fixture.requestBody(t, first, second), key)

	assertPublicContractError(t, recorder, http.StatusBadRequest, "bad_missed_session_id")
	if count := fixture.absenceCount(t); count != 0 {
		t.Fatalf("failed batch persisted %d absences, want transaction rollback to 0", count)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var idempotencyRows int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM idempotency_keys WHERE scope = 'absences-public' AND idempotency_key = $1`,
		key,
	).Scan(&idempotencyRows); err != nil {
		t.Fatalf("count idempotency rows: %v", err)
	}
	if idempotencyRows != 0 {
		t.Fatalf("failed batch retained %d idempotency rows, want 0 after rollback", idempotencyRows)
	}
}

func TestPublicBatchReplaysSameKeyWithoutDuplicatingAbsence(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)
	body := fixture.requestBody(t, fixture.validItem(0))
	key := uuid.NewString()

	firstRecorder := fixture.submitBatch(body, key)
	// A submission burns the student session, so the client re-verifies before
	// retrying. The wcode-keyed idempotency partition keeps the same key
	// replayable across the new session.
	fixture.reverify(t)
	secondRecorder := fixture.submitBatch(body, key)

	if firstRecorder.Code != http.StatusCreated || secondRecorder.Code != http.StatusCreated {
		t.Fatalf("statuses = (%d, %d), want (201, 201); first = %s; second = %s",
			firstRecorder.Code, secondRecorder.Code, firstRecorder.Body.String(), secondRecorder.Body.String())
	}
	first := decodeContractBatchResponse(t, firstRecorder)
	second := decodeContractBatchResponse(t, secondRecorder)
	if len(first.Items) != 1 || len(second.Items) != 1 {
		t.Fatalf("response item counts = (%d, %d), want (1, 1)", len(first.Items), len(second.Items))
	}
	if first.Items[0].ID == "" || second.Items[0].ID != first.Items[0].ID {
		t.Fatalf("replayed IDs = (%q, %q), want the same non-empty ID", first.Items[0].ID, second.Items[0].ID)
	}
	if count := fixture.absenceCount(t); count != 1 {
		t.Fatalf("same-key replay persisted %d absences, want 1", count)
	}
}

func TestPublicBatchConcurrentSameKeyCreatesExactlyOneAbsence(t *testing.T) {
	fixture := newPublicAbsenceContractFixture(t)
	body := fixture.requestBody(t, fixture.validItem(0))
	key := uuid.NewString()
	start := make(chan struct{})
	recorders := make([]*httptest.ResponseRecorder, 2)
	// Each worker authenticates with its own freshly verified session: the
	// first commit burns its session, and the loser's session must still be
	// valid to reach the idempotency cache and replay the winner's response.
	tokens := []string{
		seedVerifiedStudentSession(t, fixture.pool, fixture.wcode),
		seedVerifiedStudentSession(t, fixture.pool, fixture.wcode),
	}

	var workers sync.WaitGroup
	workers.Add(len(recorders))
	for i := range recorders {
		go func(index int) {
			defer workers.Done()
			<-start
			recorders[index] = fixture.submitBatchAs(body, key, tokens[index])
		}(i)
	}
	close(start)

	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("concurrent same-key submissions did not complete")
	}

	responses := make([]contractBatchResponse, len(recorders))
	for i, recorder := range recorders {
		if recorder.Code != http.StatusCreated {
			t.Fatalf("response %d status = %d, want 201; body = %s", i, recorder.Code, recorder.Body.String())
		}
		responses[i] = decodeContractBatchResponse(t, recorder)
		if len(responses[i].Items) != 1 {
			t.Fatalf("response %d item count = %d, want 1", i, len(responses[i].Items))
		}
	}
	if responses[0].Items[0].ID == "" || responses[0].Items[0].ID != responses[1].Items[0].ID {
		t.Fatalf("concurrent response IDs = (%q, %q), want the same non-empty ID", responses[0].Items[0].ID, responses[1].Items[0].ID)
	}
	if count := fixture.absenceCount(t); count != 1 {
		t.Fatalf("concurrent same-key submissions persisted %d absences, want 1", count)
	}
}
