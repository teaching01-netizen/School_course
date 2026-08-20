package studentshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"warwick-institute/internal/auth"
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/httpapi/httpadapter"
	"warwick-institute/internal/httpapi/httpdeps"
)

type routeAuth struct {
	user auth.AuthenticatedUser
	err  error
}

func (f routeAuth) RequireUser(context.Context, *http.Request) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}

func (routeAuth) HandleLogin(http.ResponseWriter, *http.Request) error  { return nil }
func (routeAuth) HandleLogout(http.ResponseWriter, *http.Request) error { return nil }

func requireStudentsTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run DB integration tests")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestStudentGetByWCodeMatchesAnyCase(t *testing.T) {
	pool := requireStudentsTestDB(t)
	q := sqldb.New(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Legacy-imported students store uppercase wcodes; CRM rows are often
	// lowercase. The by-wcode lookup must match both spellings.
	upper := "W200" + time.Now().UTC().Format("150405") + "0"
	lower := "w200" + time.Now().UTC().Format("150405") + "1"
	if _, err := pool.Exec(ctx, `INSERT INTO students (wcode, full_name) VALUES ($1, 'Upper Student'), ($2, 'Lower Student')`, upper, lower); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM students WHERE wcode = ANY($1)`, []string{upper, lower})
	})

	s := &server{
		deps: httpdeps.Deps{DB: pool, Q: q, Auth: routeAuth{user: auth.AuthenticatedUser{Role: "Admin"}}},
		a:    httpadapter.New(routeAuth{user: auth.AuthenticatedUser{Role: "Admin"}}, nil),
	}
	get := func(wcode string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/students/by-wcode?wcode="+wcode, nil)
		response := httptest.NewRecorder()
		s.handleStudentsGetByWCode(response, request)
		return response
	}

	for _, wcode := range []string{upper, lower} {
		response := get(wcode)
		if response.Code != http.StatusOK {
			t.Fatalf("by-wcode %s = %d, want 200; body = %s", wcode, response.Code, response.Body.String())
		}
	}
}