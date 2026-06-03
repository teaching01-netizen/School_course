package db

import (
	"os"
	"path/filepath"
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
