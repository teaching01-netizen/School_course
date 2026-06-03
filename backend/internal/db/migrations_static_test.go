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
