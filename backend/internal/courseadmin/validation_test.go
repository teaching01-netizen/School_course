package courseadmin

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

func testUUID(n byte) pgtype.UUID {
	var b [16]byte
	b[0] = n
	return pgtype.UUID{Bytes: b, Valid: true}
}

func assignment(id pgtype.UUID, primary bool) TeacherAssignment {
	return TeacherAssignment{TeacherID: id, IsPrimary: primary}
}

func TestValidateTeacherAssignments_EmptySet(t *testing.T) {
	if err := validateTeacherAssignments(nil); err != nil {
		t.Fatalf("empty set should be valid, got %v", err)
	}
	if err := validateTeacherAssignments([]TeacherAssignment{}); err != nil {
		t.Fatalf("empty set should be valid, got %v", err)
	}
}

func TestValidateTeacherAssignments_SingleTeacher(t *testing.T) {
	if err := validateTeacherAssignments([]TeacherAssignment{assignment(testUUID(1), false)}); err != nil {
		t.Fatalf("single teacher should be valid, got %v", err)
	}
}

func TestValidateTeacherAssignments_MultipleTeachers(t *testing.T) {
	assignments := []TeacherAssignment{
		assignment(testUUID(1), false),
		assignment(testUUID(2), false),
		assignment(testUUID(3), false),
	}
	if err := validateTeacherAssignments(assignments); err != nil {
		t.Fatalf("multiple teachers should be valid, got %v", err)
	}
}

func TestValidateTeacherAssignments_OnePrimaryAmongSeveral(t *testing.T) {
	assignments := []TeacherAssignment{
		assignment(testUUID(1), false),
		assignment(testUUID(2), true),
		assignment(testUUID(3), false),
	}
	if err := validateTeacherAssignments(assignments); err != nil {
		t.Fatalf("one primary should be valid, got %v", err)
	}
}

func TestValidateTeacherAssignments_NoPrimaryAmongSeveral(t *testing.T) {
	assignments := []TeacherAssignment{
		assignment(testUUID(1), false),
		assignment(testUUID(2), false),
	}
	if err := validateTeacherAssignments(assignments); err != nil {
		t.Fatalf("no primary should be valid, got %v", err)
	}
}

func TestValidateTeacherAssignments_Duplicate(t *testing.T) {
	dup := testUUID(1)
	err := validateTeacherAssignments([]TeacherAssignment{
		assignment(dup, false),
		assignment(dup, false),
	})
	ce, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if ce.Code != "duplicate_teacher" {
		t.Fatalf("expected duplicate_teacher, got %s", ce.Code)
	}
	if ce.Details["index"] != 1 {
		t.Fatalf("expected index 1, got %v", ce.Details["index"])
	}
}

func TestValidateTeacherAssignments_TwoPrimaries(t *testing.T) {
	err := validateTeacherAssignments([]TeacherAssignment{
		assignment(testUUID(1), true),
		assignment(testUUID(2), true),
	})
	ce, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if ce.Code != "multiple_primary_teachers" {
		t.Fatalf("expected multiple_primary_teachers, got %s", ce.Code)
	}
}

func TestValidateTeacherAssignments_InvalidUUID(t *testing.T) {
	err := validateTeacherAssignments([]TeacherAssignment{
		assignment(testUUID(1), false),
		assignment(pgtype.UUID{Valid: false}, true),
	})
	ce, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if ce.Code != "invalid_teacher" {
		t.Fatalf("expected invalid_teacher, got %s", ce.Code)
	}
	if ce.Details["index"] != 1 {
		t.Fatalf("expected index 1, got %v", ce.Details["index"])
	}
	if ce.Details["reason"] != "invalid_id" {
		t.Fatalf("expected reason invalid_id, got %v", ce.Details["reason"])
	}
}

func TestValidateTeacherAssignments_MoreThanMax(t *testing.T) {
	assignments := make([]TeacherAssignment, 0, MaxTeachersPerCourse+1)
	for i := 0; i < MaxTeachersPerCourse+1; i++ {
		assignments = append(assignments, assignment(testUUID(byte(i+1)), false))
	}
	err := validateTeacherAssignments(assignments)
	ce, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if ce.Code != "too_many_teachers" {
		t.Fatalf("expected too_many_teachers, got %s", ce.Code)
	}
	if ce.Details["maximum"] != MaxTeachersPerCourse {
		t.Fatalf("expected maximum %d, got %v", MaxTeachersPerCourse, ce.Details["maximum"])
	}
	if ce.Details["received"] != MaxTeachersPerCourse+1 {
		t.Fatalf("expected received %d, got %v", MaxTeachersPerCourse+1, ce.Details["received"])
	}
}

func TestValidateTeacherAssignments_ExactlyMax(t *testing.T) {
	assignments := make([]TeacherAssignment, 0, MaxTeachersPerCourse)
	for i := 0; i < MaxTeachersPerCourse; i++ {
		assignments = append(assignments, assignment(testUUID(byte(i+1)), false))
	}
	if err := validateTeacherAssignments(assignments); err != nil {
		t.Fatalf("exactly max teachers should be valid, got %v", err)
	}
}

func TestPrimaryTeacherID_ReturnsPrimary(t *testing.T) {
	assignments := []TeacherAssignment{
		assignment(testUUID(1), false),
		assignment(testUUID(2), true),
	}
	got := primaryTeacherID(assignments)
	if !got.Valid || got.Bytes != testUUID(2).Bytes {
		t.Fatalf("expected primary 2, got %v", got)
	}
}

func TestPrimaryTeacherID_NoPrimary(t *testing.T) {
	assignments := []TeacherAssignment{
		assignment(testUUID(1), false),
		assignment(testUUID(2), false),
	}
	got := primaryTeacherID(assignments)
	if got.Valid {
		t.Fatalf("expected invalid primary, got %v", got)
	}
}

func existingRow(id pgtype.UUID, primary bool, username string) sqldb.CourseTeachersListRow {
	return sqldb.CourseTeachersListRow{
		CourseID:  testUUID(99),
		TeacherID: id,
		IsPrimary: primary,
		Username:  username,
	}
}

func idsOnly(rows []sqldb.CourseTeachersListRow) map[[16]byte]bool {
	out := make(map[[16]byte]bool, len(rows))
	for _, r := range rows {
		out[r.TeacherID.Bytes] = true
	}
	return out
}

func TestCalculateRemovedTeacherIDs_NoChange(t *testing.T) {
	existing := []sqldb.CourseTeachersListRow{
		existingRow(testUUID(1), true, "a"),
		existingRow(testUUID(2), false, "b"),
	}
	assignments := []TeacherAssignment{
		assignment(testUUID(1), true),
		assignment(testUUID(2), false),
	}
	removed := calculateRemovedTeacherIDs(existing, assignments)
	if len(removed) != 0 {
		t.Fatalf("expected no removals, got %v", removed)
	}
}

func TestCalculateRemovedTeacherIDs_Add(t *testing.T) {
	existing := []sqldb.CourseTeachersListRow{
		existingRow(testUUID(1), true, "a"),
	}
	assignments := []TeacherAssignment{
		assignment(testUUID(1), true),
		assignment(testUUID(2), false),
	}
	removed := calculateRemovedTeacherIDs(existing, assignments)
	if len(removed) != 0 {
		t.Fatalf("adding a teacher must not mark removals, got %v", removed)
	}
}

func TestCalculateRemovedTeacherIDs_Remove(t *testing.T) {
	existing := []sqldb.CourseTeachersListRow{
		existingRow(testUUID(1), true, "a"),
		existingRow(testUUID(2), false, "b"),
		existingRow(testUUID(3), false, "c"),
	}
	assignments := []TeacherAssignment{
		assignment(testUUID(1), true),
	}
	removed := calculateRemovedTeacherIDs(existing, assignments)
	if len(removed) != 2 {
		t.Fatalf("expected 2 removals, got %v", removed)
	}
	got := make(map[[16]byte]bool)
	for _, id := range removed {
		got[id.Bytes] = true
	}
	if !got[testUUID(2).Bytes] || !got[testUUID(3).Bytes] {
		t.Fatalf("expected teachers 2 and 3 removed, got %v", removed)
	}
}

func TestCalculateRemovedTeacherIDs_ReplaceAll(t *testing.T) {
	existing := []sqldb.CourseTeachersListRow{
		existingRow(testUUID(1), true, "a"),
		existingRow(testUUID(2), false, "b"),
	}
	assignments := []TeacherAssignment{
		assignment(testUUID(3), true),
		assignment(testUUID(4), false),
	}
	removed := calculateRemovedTeacherIDs(existing, assignments)
	if len(removed) != 2 {
		t.Fatalf("expected both old teachers removed, got %v", removed)
	}
}

func TestCalculateRemovedTeacherIDs_PrimaryOnlyChange(t *testing.T) {
	existing := []sqldb.CourseTeachersListRow{
		existingRow(testUUID(1), true, "a"),
		existingRow(testUUID(2), false, "b"),
	}
	assignments := []TeacherAssignment{
		assignment(testUUID(1), false),
		assignment(testUUID(2), true),
	}
	removed := calculateRemovedTeacherIDs(existing, assignments)
	if len(removed) != 0 {
		t.Fatalf("primary swap must not count as removal, got %v", removed)
	}
}

func TestCalculateRemovedTeacherIDs_Reorder(t *testing.T) {
	existing := []sqldb.CourseTeachersListRow{
		existingRow(testUUID(1), true, "a"),
		existingRow(testUUID(2), false, "b"),
	}
	assignments := []TeacherAssignment{
		assignment(testUUID(2), false),
		assignment(testUUID(1), true),
	}
	removed := calculateRemovedTeacherIDs(existing, assignments)
	if len(removed) != 0 {
		t.Fatalf("reordering must not count as removal, got %v", removed)
	}
}

func TestCalculateRemovedTeacherIDs_EmptyExisting(t *testing.T) {
	assignments := []TeacherAssignment{
		assignment(testUUID(1), false),
	}
	if removed := calculateRemovedTeacherIDs(nil, assignments); len(removed) != 0 {
		t.Fatalf("no existing assignments means no removals, got %v", removed)
	}
}

func TestHTTPStatusForError_MappingTable(t *testing.T) {
	cases := []struct {
		name string
		err  *Error
		want int
	}{
		{"invalid_teacher", &Error{Code: "invalid_teacher"}, http.StatusBadRequest},
		{"too_many_teachers", &Error{Code: "too_many_teachers"}, http.StatusBadRequest},
		{"duplicate_teacher", &Error{Code: "duplicate_teacher"}, http.StatusBadRequest},
		{"multiple_primary_teachers", &Error{Code: "multiple_primary_teachers"}, http.StatusBadRequest},
		{"invalid_expected_version", &Error{Code: "invalid_expected_version"}, http.StatusBadRequest},
		{"teacher_in_use", &Error{Code: "teacher_in_use"}, http.StatusConflict},
		{"stale_edit", &Error{Code: "stale_edit"}, http.StatusConflict},
		{"not_found", &Error{Code: "not_found"}, http.StatusNotFound},
		{"unknown", &Error{Code: "something_else"}, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HTTPStatusForError(tc.err); got != tc.want {
				t.Fatalf("HTTPStatusForError(%s) = %d, want %d", tc.err.Code, got, tc.want)
			}
		})
	}
}
