package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type adminUserListRecordingDB struct {
	query string
	args  []interface{}
}

func (f *adminUserListRecordingDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *adminUserListRecordingDB) Query(_ context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	f.query = query
	f.args = args
	return &adminUserListEmptyRows{}, nil
}

func (f *adminUserListRecordingDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return nil
}

type adminUserListEmptyRows struct {
	closed bool
}

func (r *adminUserListEmptyRows) Close()                                       { r.closed = true }
func (r *adminUserListEmptyRows) Err() error                                   { return nil }
func (r *adminUserListEmptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *adminUserListEmptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *adminUserListEmptyRows) Next() bool                                   { return false }
func (r *adminUserListEmptyRows) Scan(...interface{}) error                    { return pgx.ErrNoRows }
func (r *adminUserListEmptyRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *adminUserListEmptyRows) RawValues() [][]byte                          { return nil }
func (r *adminUserListEmptyRows) Conn() *pgx.Conn                              { return nil }

func TestAdminUserListBindsSearchAsFirstParameter(t *testing.T) {
	for _, includeDeleted := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "include deleted"}[includeDeleted], func(t *testing.T) {
			db := &adminUserListRecordingDB{}
			_, err := New(db).AdminUserList(context.Background(), AdminUserListParams{
				IncludeDeleted: includeDeleted,
				Search:         "admin",
			})
			if err != nil {
				t.Fatalf("AdminUserList() error = %v", err)
			}
			if len(db.args) != 1 || db.args[0] != "admin" {
				t.Fatalf("query args = %#v, want one search argument", db.args)
			}
			if !strings.Contains(db.query, "$1") || strings.Contains(db.query, "$2") {
				t.Fatalf("query must bind its only argument as $1, got:\n%s", db.query)
			}
		})
	}
}

func TestAdminUserListQueriesPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pools, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer pools.Close()

	if err := pools.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	if _, err := New(pools).AdminUserList(ctx, AdminUserListParams{Search: "admin"}); err != nil {
		t.Fatalf("AdminUserList() against PostgreSQL: %v", err)
	}
}
