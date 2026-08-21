package db

import (
	"strings"
	"testing"
)

func TestCourseExpiryMigrationWrapsDollarQuotedBlock(t *testing.T) {
	sql := readMigration(t, "00101_course_expiry_and_cycle_config.sql")

	if !strings.Contains(sql, "-- +goose StatementBegin\nDO $$") ||
		!strings.Contains(sql, "END $$;\n-- +goose StatementEnd") {
		t.Fatal("00101 must wrap its DO $$ block with Goose StatementBegin/StatementEnd")
	}
}
