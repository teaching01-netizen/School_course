package absenceshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/studentauth"
)

type sitInPolicyCommitBarrier struct {
	ready   chan uint32
	release <-chan struct{}
}

func (b sitInPolicyCommitBarrier) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.EqualFold(strings.TrimSpace(data.SQL), "commit") {
		b.ready <- conn.PgConn().PID()
		select {
		case <-b.release:
		case <-ctx.Done():
		}
	}
	return ctx
}

func (sitInPolicyCommitBarrier) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {
}

func TestSuggestedSitInSerializesWithPolicyDisable(t *testing.T) {
	for _, tc := range []struct {
		name, first string
		student     bool
	}{
		{"staff_booking_commits_first", "booking", false},
		{"staff_policy_commits_first", "policy", false},
		{"student_booking_commits_first", "booking", true},
		{"student_policy_commits_first", "policy", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := seedFinalSessionFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			release := make(chan struct{})
			unblock := func() {
				if release != nil {
					close(release)
					release = nil
				}
			}
			defer unblock()
			barrier := sitInPolicyCommitBarrier{ready: make(chan uint32, 1), release: release}
			config, err := pgxpool.ParseConfig(requireStaffTestDB(t))
			if err != nil {
				t.Fatal(err)
			}
			config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
			config.ConnConfig.Tracer = barrier
			pool, err := pgxpool.NewWithConfig(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pool.Close)
			paused := f
			paused.pool, paused.q = pool, sqldb.New(pool)
			pausedServer, peerServer := paused.staffServer(t), f.staffServer(t)
			bookingHandler, policyHandler := pausedServer.Config.Handler, peerServer.Config.Handler
			bookingPool := pool
			if tc.first == "policy" {
				bookingHandler, policyHandler = policyHandler, bookingHandler
				bookingPool = f.pool
			}
			var token string
			if tc.student {
				token = seedVerifiedStudentSession(t, f.pool, f.wcode)
				bookingHandler = selfServiceMux(t, bookingPool)
			}

			submit := func(handler http.Handler, method, path string, body any, batch bool) <-chan *httptest.ResponseRecorder {
				t.Helper()
				payload, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", uuid.NewString())
				if batch {
					request.Header.Set("X-Staff-Batch", "true")
				}
				if path == "/api/v1/absences/batch" {
					request.AddCookie(&http.Cookie{Name: studentauth.CookieName(false), Value: token})
				}
				result := make(chan *httptest.ResponseRecorder, 1)
				go func() {
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					result <- response
				}()
				return result
			}
			book := func() <-chan *httptest.ResponseRecorder {
				item := f.requestBody()
				item["sit_in_session_ids"] = []string{f.earlierID}
				if tc.student {
					delete(item, "wcode")
					return submit(bookingHandler, http.MethodPost, "/api/v1/absences/batch",
						map[string]any{"reason": "Medical appointment", "items": []any{item}}, false)
				}
				return submit(bookingHandler, http.MethodPost, "/api/v1/absences/staff-create",
					map[string]any{"items": []any{item}}, true)
			}
			disable := func() <-chan *httptest.ResponseRecorder {
				body := map[string]any{"absence_policies": map[string]any{"sit_in": map[string]bool{"auto_resolve_enabled": false}}}
				return submit(policyHandler, http.MethodPut, "/api/v1/admin/absence-policies", body, false)
			}
			await := func(result <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
				t.Helper()
				select {
				case response := <-result:
					return response
				case <-ctx.Done():
					t.Fatal(ctx.Err())
					return nil
				}
			}

			var booking, policy <-chan *httptest.ResponseRecorder
			if tc.first == "booking" {
				booking = book()
			} else {
				policy = disable()
			}
			var blocker uint32
			select {
			case blocker = <-barrier.ready:
			case <-ctx.Done():
				t.Fatalf("%s never reached COMMIT: %v", tc.first, ctx.Err())
			}

			var waiting <-chan *httptest.ResponseRecorder
			if tc.first == "booking" {
				policy = disable()
				waiting = policy
			} else {
				booking = book()
				waiting = booking
			}
			for {
				var blocked bool
				if err := f.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)))`, blocker).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case response := <-waiting:
					t.Fatalf("second request completed before %s committed: %d %s", tc.first, response.Code, response.Body)
				case <-time.After(10 * time.Millisecond):
				}
			}

			unblock()
			policyResponse := await(policy)
			assertFinalSessionResponse(t, policyResponse.Code, http.StatusOK, policyResponse.Body.Bytes())
			bookingResponse := await(booking)
			if tc.first == "booking" {
				assertFinalSessionResponse(t, bookingResponse.Code, http.StatusCreated, bookingResponse.Body.Bytes())
				f.assertPersisted(t, f.earlierID)
				return
			}
			assertFinalSessionResponse(t, bookingResponse.Code, http.StatusBadRequest, bookingResponse.Body.Bytes())
			if !strings.Contains(bookingResponse.Body.String(), `"code":"invalid_sessions"`) {
				t.Fatalf("expected invalid_sessions, got %s", bookingResponse.Body)
			}
			var count int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM student_absences WHERE wcode = $1`, f.wcode).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("rejected booking persisted %d absences", count)
			}
		})
	}
}
