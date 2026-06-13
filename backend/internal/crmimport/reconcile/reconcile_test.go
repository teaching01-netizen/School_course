package reconcile

import "testing"

func TestCRMConflictCourseDisplayNamePrefersSubjectName(t *testing.T) {
	course := CRMConflictCourse{
		ID:          "course-1",
		Code:        "SAT",
		Name:        "SAT Math Course",
		SubjectName: "SAT Math",
	}

	if got := course.displayName(); got != "SAT Math" {
		t.Fatalf("displayName() = %q, want subject name", got)
	}
}

func TestCRMConflictCourseDisplayNameFallbacks(t *testing.T) {
	cases := []struct {
		name   string
		course CRMConflictCourse
		want   string
	}{
		{
			name:   "course name",
			course: CRMConflictCourse{ID: "course-1", Code: "SAT", Name: "SAT Math Course"},
			want:   "SAT Math Course",
		},
		{
			name:   "course code",
			course: CRMConflictCourse{ID: "course-1", Code: "SAT"},
			want:   "SAT",
		},
		{
			name:   "course id",
			course: CRMConflictCourse{ID: "course-1"},
			want:   "course-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.course.displayName(); got != tc.want {
				t.Fatalf("displayName() = %q, want %q", got, tc.want)
			}
		})
	}
}
