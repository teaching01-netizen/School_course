package db

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func readMigration(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations", name))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(data)
}

func TestHardDeleteMigrationUsesExistingStudentAbsencesSubjectFK(t *testing.T) {
	sql := readMigration(t, "00032_hard_delete_courses_subjects_rooms.sql")

	if strings.Contains(sql, "absence_extensions") {
		t.Fatal("00032 must not reference absence_extensions; subject_id lives on student_absences")
	}
	if !strings.Contains(sql, "ALTER TABLE student_absences") ||
		!strings.Contains(sql, "student_absences_subject_id_fkey") ||
		!strings.Contains(sql, "FOREIGN KEY (subject_id) REFERENCES subjects(id) ON DELETE CASCADE") {
		t.Fatal("00032 must cascade student_absences.subject_id to subjects(id)")
	}
}

func TestBackfillMigration_FillsNullStudentAndParentPhones(t *testing.T) {
	sql := readMigration(t, "00034_backfill_student_phones.sql")

	if !strings.Contains(sql, "UPDATE students") {
		t.Fatal("00034 must UPDATE students")
	}
	if !strings.Contains(sql, "crm_rows") {
		t.Fatal("00034 must reference crm_rows")
	}
	if !strings.Contains(sql, "student_phone") {
		t.Fatal("00034 must backfill student_phone")
	}
	if !strings.Contains(sql, "parent_phone") {
		t.Fatal("00034 must backfill parent_phone")
	}
	if !strings.Contains(sql, "NULL OR btrim") {
		t.Fatal("00034 must only update NULL/empty phones")
	}
}

func TestSatVerbalPolicyMappingsFixWrapsDollarQuotedBlock(t *testing.T) {
	sql := readMigration(t, "00039_fix_sat_verbal_policy_mappings_course_id.sql")

	if !strings.Contains(sql, "-- +goose StatementBegin\nDO $$") ||
		!strings.Contains(sql, "END $$;\n-- +goose StatementEnd") {
		t.Fatal("00039 must wrap its DO $$ block with goose StatementBegin/StatementEnd")
	}
}

func TestSatVerbalPolicyMappingsRepairAddsAllRuntimeColumns(t *testing.T) {
	for _, name := range []string{
		"00039_fix_sat_verbal_policy_mappings_course_id.sql",
		"00040_repair_sat_verbal_policy_mappings_schema.sql",
	} {
		sql := readMigration(t, name)
		for _, column := range []string{
			"id",
			"rule_id",
			"course_id",
			"policy_rule",
			"policy_hash",
			"active",
			"created_at",
			"updated_at",
		} {
			if !strings.Contains(sql, "column_name = '"+column+"'") {
				t.Fatalf("%s must repair missing %s column", name, column)
			}
		}
		if !strings.Contains(sql, "DELETE FROM sat_verbal_policy_mappings") ||
			!strings.Contains(sql, "WHERE course_id IS NULL") {
			t.Fatalf("%s must remove unrecoverable rows before enforcing course_id NOT NULL", name)
		}
		if !strings.Contains(sql, "sat_verbal_policy_mappings_rule_id_unique UNIQUE (rule_id)") {
			t.Fatalf("%s must restore rule_id uniqueness", name)
		}
	}
}

func TestCodeDoesNotQueryDroppedCourseOrSubjectDeletedAtColumns(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	backendDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	paths := []string{
		filepath.Join(backendDir, "db", "queries"),
		filepath.Join(backendDir, "internal"),
	}
	stalePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)\bcourses\s*\.\s*deleted_at\b`),
		regexp.MustCompile(`(?is)\bsubjects\s*\.\s*deleted_at\b`),
		regexp.MustCompile(`(?is)\bfrom\s+courses\s+c\b.*?\bc\s*\.\s*deleted_at\b`),
		regexp.MustCompile(`(?is)\bfrom\s+subjects\s+s\b.*?\bs\s*\.\s*deleted_at\b`),
		regexp.MustCompile(`(?im)^\s*from\s+courses\s*$\s*^\s*where[^\n]*\bdeleted_at\b`),
		regexp.MustCompile(`(?im)^\s*from\s+subjects\s*$\s*^\s*where[^\n]*\bdeleted_at\b`),
		regexp.MustCompile(`(?im)^\s*join\s+courses\s+\w+\s+on[^\n]*\bdeleted_at\b`),
		regexp.MustCompile(`(?im)^\s*join\s+subjects\s+\w+\s+on[^\n]*\bdeleted_at\b`),
	}

	for _, root := range paths {
		if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".go" && filepath.Ext(path) != ".sql" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			for _, pattern := range stalePatterns {
				if pattern.MatchString(text) {
					t.Errorf("%s references dropped courses/subjects deleted_at column with pattern %q", path, pattern.String())
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}
