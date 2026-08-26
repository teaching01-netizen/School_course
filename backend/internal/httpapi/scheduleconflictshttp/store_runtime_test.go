package scheduleconflictshttp

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConflictStoreQueryExecutesAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	got, err := (conflictStore{db: pool}).list(ctx, listFilters{Limit: 50})
	if err != nil {
		t.Fatalf("optimized conflict query failed: %v", err)
	}
	if got.Limit != 50 || got.Offset != 0 || got.TotalCount < len(got.Items) {
		t.Fatalf("invalid pagination metadata: %+v", got)
	}
}
