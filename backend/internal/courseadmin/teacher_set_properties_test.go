package courseadmin

import (
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestTeacherSetInvariants(t *testing.T) {
	f := setupTestDB(t)
	svc := NewService()
	var (
		seed1 uint64 = uint64(time.Now().UnixNano())
		seed2 uint64 = seed1 + 1
	)
	rng := rand.New(rand.NewPCG(seed1, seed2))

	// Pre-create a pool of teacher IDs and a course
	teacherPool := make([]pgtype.UUID, 20)
	for i := range teacherPool {
		teacherPool[i] = f.createTeacher(t, "Teacher")
	}
	actorID := f.createTeacher(t, "Admin")
	courseID := f.createCourse(t, "INV-"+f.suffix)

	// Helper: random teacher assignment (subset, with optional primary)
	randomAssignments := func(count int) ([]TeacherAssignment, map[[16]byte]bool) {
		if count == 0 {
			return []TeacherAssignment{}, map[[16]byte]bool{}
		}

		// Pick `count` distinct teachers from the pool
		perm := rng.Perm(len(teacherPool))
		chosen := perm[:count]

		assignments := make([]TeacherAssignment, count)
		expected := make(map[[16]byte]bool, count)
		hasPrimary := false

		for i, idx := range chosen {
			isPrimary := false
			if !hasPrimary && (i == 0 || rng.Float32() < 0.3) {
				isPrimary = true
				hasPrimary = true
			}
			assignments[i] = TeacherAssignment{TeacherID: teacherPool[idx], IsPrimary: isPrimary}
			expected[teacherPool[idx].Bytes] = isPrimary
		}

		return assignments, expected
	}

	// Run many random update cycles
	const cycles = 50
	currentVersion := int32(1)

	for i := range cycles {
		count := rng.IntN(6) // 0-5 teachers each cycle
		assignments, expected := randomAssignments(count)

		cmd := UpdateCourseCommand{
			CourseID:        courseID,
			ActorID:         actorID,
			ExpectedVersion: currentVersion,
			Code:            "INV-" + f.suffix,
			Name:            "Invariant test",
			Teachers:        assignments,
		}

		result, err := f.runUpdate(t, svc, cmd)
		if err != nil {
			t.Fatalf("cycle %d: update failed: %v", i, err)
		}

		// Invariant 1: version increments by exactly 1
		if result.Version != currentVersion+1 {
			t.Fatalf("cycle %d: expected version %d, got %d", i, currentVersion+1, result.Version)
		}
		currentVersion = result.Version

		// Invariant 2: stored set exactly equals submitted set
		stored := f.courseTeacherIDs(t, courseID)
		if len(stored) != len(expected) {
			t.Fatalf("cycle %d: expected %d teachers, got %d: stored=%v expected=%v",
				i, len(expected), len(stored), stored, expected)
		}
		for id, isPrimary := range expected {
			storedPrimary, ok := stored[id]
			if !ok {
				t.Fatalf("cycle %d: teacher %x missing from stored set", i, id)
			}
			if storedPrimary != isPrimary {
				t.Fatalf("cycle %d: teacher %x primary mismatch: stored=%v expected=%v",
					i, id, storedPrimary, isPrimary)
			}
		}

		// Invariant 3: no duplicate IDs in stored set (structural guarantee from map)
		// Invariant 4: at most one primary
		primaryCount := 0
		for _, isPrimary := range stored {
			if isPrimary {
				primaryCount++
			}
		}
		if primaryCount > 1 {
			t.Fatalf("cycle %d: %d primary teachers (max 1)", i, primaryCount)
		}
	}

	// Invariant 5: invalid ExpectedVersion must fail and leave state unchanged
	savedVersion := currentVersion

	// Values guaranteed to never match the actual version:
	// 0 and -1 trigger invalid_expected_version (<= 0 check),
	// savedVersion+100 triggers stale_edit (version mismatch).
	badVersions := []int32{0, -1, savedVersion + 100}
	for _, badVer := range badVersions {
		err := f.mustUpdateErr(t, svc, courseID, UpdateCourseCommand{
			CourseID:        courseID,
			ActorID:         actorID,
			ExpectedVersion: badVer,
			Code:            "INV-" + f.suffix,
			Name:            "Invariant test",
			Teachers:        []TeacherAssignment{{TeacherID: teacherPool[0], IsPrimary: true}},
		})
		// Verify the error is a domain error (either invalid_expected_version or stale_edit)
		var ce *Error
		if !errors.As(err, &ce) {
			t.Fatalf("invalid version %d returned non-domain error: %v", badVer, err)
		}
	}

	// Invariant 6: version unchanged despite all failed attempts.
	// Since version and teacher set are updated atomically, an unchanged version
	// guarantees the teacher set is also untouched.
	if v := f.courseVersion(t, courseID); v != savedVersion {
		t.Fatalf("version changed from %d to %d after failed updates", savedVersion, v)
	}
}
