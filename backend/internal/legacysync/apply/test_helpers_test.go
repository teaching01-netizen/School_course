package apply

import (
	"github.com/jackc/pgx/v5/pgxpool"

	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/schedulepolicy"
)

func newTestScheduleApplier(pool *pgxpool.Pool, q *sqldb.Queries, source string) *ScheduleApplier {
	return NewScheduleApplier(pool, q, source, schedulepolicy.NewDBReader())
}

func newTestCourseApplier(pool *pgxpool.Pool, q *sqldb.Queries, source string) *CourseApplier {
	return NewCourseApplier(pool, q, source, schedulepolicy.NewDBReader())
}
