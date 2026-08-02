package scheduling

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	sqldb "warwick-institute/internal/db"
)

// membershipCase is a single table-driven test case for preflightSlot resource
// and membership validation. Each case defines the QueryRow results that
// CourseTeacherMembershipGet and SchedulingResourcesGet should return, and
// asserts the resulting *Err code.
type membershipCase struct {
	name           string
	input          preflightInput
	membershipVals []bool // scan values for CourseTeacherMembershipGet (3 bools)
	resourcesVals  []bool // scan values for SchedulingResourcesGet (4 bools)
	roomIDValid    bool   // whether the room input has a valid UUID
	wantErr        bool   // true if se should be non-nil
	wantCode       string // expected se.Code when wantErr
}

func TestPreflightSlot_MembershipValidation(t *testing.T) {
	ctx := context.Background()
	baseInput := testPreflightInput()

	tests := []membershipCase{
		{
			name:           "Course missing",
			input:          baseInput,
			membershipVals: []bool{false, false, false},
			wantErr:        true,
			wantCode:       ErrCourseNotFound,
		},
		{
			name:           "Course exists but empty teacher set",
			input:          baseInput,
			membershipVals: []bool{true, false, false},
			resourcesVals:  []bool{true, true, true, true}, // resources valid to pass through
			wantErr:        true,
			wantCode:       ErrCourseHasNoTeachers,
		},
		{
			name:           "Teacher not assigned to course",
			input:          baseInput,
			membershipVals: []bool{true, true, false},
			wantErr:        true,
			wantCode:       ErrTeacherNotAssigned,
		},
		{
			name:           "Teacher assigned to course — passes membership",
			input:          baseInput,
			membershipVals: []bool{true, true, true},
			resourcesVals:  []bool{true, true, true, true}, // all resources valid
			wantErr:        false,
			wantCode:       "",
		},
		{
			name:           "Teacher inactive",
			input:          baseInput,
			membershipVals: []bool{true, true, true},
			resourcesVals:  []bool{true, true, false, true}, // teacher_active = false
			wantErr:        true,
			wantCode:       ErrTeacherInactive,
		},
		{
			name:           "Room not found",
			input:          baseInput,
			membershipVals: []bool{true, true, true},
			resourcesVals:  []bool{true, true, true, false}, // room_exists = false
			roomIDValid:    true,
			wantErr:        true,
			wantCode:       ErrRoomNotFound,
		},
	}

	svc := &Service{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDBTX{}

			// CourseTeacherMembershipGet at idx 0.
			db.queryRowResults = append(db.queryRowResults, &fakeMultiRow{vals: tc.membershipVals})

			// SchedulingResourcesGet at idx 1.
			db.queryRowResults = append(db.queryRowResults, &fakeMultiRow{vals: tc.resourcesVals})

			// Default availability/overlap results: teacher available, room available.
			db.queryRowResults = append(db.queryRowResults, &fakeMultiRow{vals: []bool{true, true}}) // IsTeacherAvailable
			db.queryRowResults = append(db.queryRowResults, &fakeMultiRow{vals: []bool{true, true}}) // IsRoomAvailable

			q := sqldb.New(db)
			in := tc.input

			// If roomIDValid is set, ensure the input has a valid room (existing testPreflightInput already has one).
			if !tc.roomIDValid {
				// Make a copy with invalid room.
				cp := in
				cp.RoomID = pgtype.UUID{Valid: false}
				in = cp
			}

			se, err := svc.preflightSlot(ctx, db, q, in)

			if err != nil {
				t.Fatalf("unexpected infrastructure error: %v", err)
			}

			if tc.wantErr {
				if se == nil {
					t.Fatal("expected scheduling error, got nil")
				}
				if se.Code != tc.wantCode {
					t.Fatalf("expected code %q, got %q", tc.wantCode, se.Code)
				}
			} else {
				if se != nil {
					t.Fatalf("expected no scheduling error, got code=%q msg=%q", se.Code, se.Message)
				}
			}
		})
	}
}
