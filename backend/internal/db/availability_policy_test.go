package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestAvailabilityPolicy_Alignment(t *testing.T) {
	databaseURL := requireTestDB(t)
	migrateUpOnce(t, databaseURL)
	dbpool := newPool(t, databaseURL)
	t.Cleanup(dbpool.Close)
	q := New(dbpool)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	teacherID, err := q.AdminUserCreate(ctx, AdminUserCreateParams{
		Username:     "teacher-policy-" + suffix,
		Role:         "Teacher",
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}

	room, err := q.RoomCreate(ctx, RoomCreateParams{
		Name:     "R-policy-" + suffix,
		Capacity: pgtype.Int4{Int32: 10, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	course, err := q.CourseCreate(ctx, CourseCreateParams{
		Code: "C-policy-" + suffix,
		Name: "Course Policy",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initially, no availability windows exist for either teacher or room.
	// Both Go preflight checks and DB inserts should succeed (default to open availability).
	t.Run("DefaultOpenAvailability", func(t *testing.T) {
		start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
		end := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)

		// Preflight checks
		tAvailable, err := q.IsTeacherAvailable(ctx, teacherID, start, end)
		if err != nil {
			t.Fatal(err)
		}
		if !tAvailable {
			t.Error("expected teacher to be available by default")
		}

		rAvailable, err := q.IsRoomAvailable(ctx, room.ID, start, end)
		if err != nil {
			t.Fatal(err)
		}
		if !rAvailable {
			t.Error("expected room to be available by default")
		}

		// DB write gate
		sess, err := q.SessionCreate(ctx, SessionCreateParams{
			CourseID:  course.ID,
			RoomID:    room.ID,
			TeacherID: teacherID,
			StartAt:   pgtype.Timestamptz{Time: start, Valid: true},
			EndAt:     pgtype.Timestamptz{Time: end, Valid: true},
		})
		if err != nil {
			t.Fatalf("expected session creation to succeed, got: %v", err)
		}

		// Cleanup session
		_, err = dbpool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", sess.ID)
		if err != nil {
			t.Fatal(err)
		}
	})

	// Define availability window: June 1, 2026 09:00 -> 12:00 UTC
	winStart := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	winEnd := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Create teacher availability window
	_, err = q.CreateTeacherAvailability(ctx, CreateTeacherAvailabilityParams{
		TeacherID: teacherID,
		StartAt:   pgtype.Timestamptz{Time: winStart, Valid: true},
		EndAt:     pgtype.Timestamptz{Time: winEnd, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create room availability window
	_, err = q.CreateRoomAvailability(ctx, CreateRoomAvailabilityParams{
		RoomID:  room.ID,
		StartAt: pgtype.Timestamptz{Time: winStart, Valid: true},
		EndAt:   pgtype.Timestamptz{Time: winEnd, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Test Cases
	tests := []struct {
		name          string
		start         time.Time
		end           time.Time
		expectSuccess bool
		errorContains string
	}{
		{
			name:          "FullyInsideWindow",
			start:         time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
			end:           time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "BordersExactlyWindow",
			start:         time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC),
			end:           time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			expectSuccess: true,
		},
		{
			name:          "OverlapsStartOfWindow",
			start:         time.Date(2026, 6, 1, 8, 30, 0, 0, time.UTC),
			end:           time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
			expectSuccess: false,
			errorContains: "not available for requested time", // trigger exception message
		},
		{
			name:          "OverlapsEndOfWindow",
			start:         time.Date(2026, 6, 1, 11, 30, 0, 0, time.UTC),
			end:           time.Date(2026, 6, 1, 12, 30, 0, 0, time.UTC),
			expectSuccess: false,
			errorContains: "not available for requested time",
		},
		{
			name:          "CompletelyOutsideWindow",
			start:         time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC),
			end:           time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC),
			expectSuccess: false,
			errorContains: "not available for requested time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A. Test Go Preflight Checks
			tAvailable, err := q.IsTeacherAvailable(ctx, teacherID, tt.start, tt.end)
			if err != nil {
				t.Fatal(err)
			}
			rAvailable, err := q.IsRoomAvailable(ctx, room.ID, tt.start, tt.end)
			if err != nil {
				t.Fatal(err)
			}

			if tt.expectSuccess {
				if !tAvailable {
					t.Error("expected teacher to be available")
				}
				if !rAvailable {
					t.Error("expected room to be available")
				}
			} else {
				// If not available, at least one check should be false
				if tAvailable && rAvailable {
					t.Error("expected at least one availability check to fail")
				}
			}

			// B. Test DB Write Gate (Trigger)
			sess, err := q.SessionCreate(ctx, SessionCreateParams{
				CourseID:  course.ID,
				RoomID:    room.ID,
				TeacherID: teacherID,
				StartAt:   pgtype.Timestamptz{Time: tt.start, Valid: true},
				EndAt:     pgtype.Timestamptz{Time: tt.end, Valid: true},
			})

			if tt.expectSuccess {
				if err != nil {
					t.Fatalf("expected DB insert to succeed, got error: %v", err)
				}
				// Cleanup
				_, err = dbpool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", sess.ID)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("expected DB insert to fail with trigger violation, but it succeeded")
				}
				var pgErr *pgconn.PgError
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error message to contain %q, got: %v", tt.errorContains, err)
				}
				// Verify it's a constraint violation code (check constraint or trigger raised exception)
				if strings.Contains(err.Error(), "pgconn.PgError") {
					pgErr = err.(*pgconn.PgError)
					if pgErr.Code != "23514" {
						t.Errorf("expected pgx error code 23514, got: %s", pgErr.Code)
					}
				}
			}
		})
	}
}
