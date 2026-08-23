import type { Meta, StoryObj } from "@storybook/react";
import { MemoryRouter } from "react-router-dom";
import { SessionAvailabilityStatus } from "./AvailabilityStatus";
import type { ConflictDetails } from "@/features/scheduling/types";
import type { Course } from "@/features/courses/types";
import type { User } from "@/types/shared";
import type { Room } from "@/features/scheduling/types";
import type { UsePreflightReturn } from "@/features/scheduling/hooks/usePreflight";

/**
 * Every visible state of the availability strip that sits inside the session
 * editor and the Add… popover. The blocked stories show the reason-first
 * conflict explanation: who/what clashes is named up front, the requested
 * window is restated, clash chips show the other classes, and the path forward
 * (suggestion + slot finder) closes the loop.
 */
const meta = {
  title: "Scheduling/AvailabilityStatus",
  component: SessionAvailabilityStatus,
  parameters: {
    docs: {
      description: {
        component:
          "Compact availability summary for the session editor. Idle, checking, available and provisional stay calm; blocked states explain the conflict by naming the actual room / teacher / students, restating what was requested, and offering a concrete next step.",
      },
    },
  },
  decorators: [
    (Story) => (
      <MemoryRouter initialEntries={["/courses/course-1"]}>
        <div className="w-[23rem]">{Story()}</div>
      </MemoryRouter>
    ),
  ],
} satisfies Meta<typeof SessionAvailabilityStatus>;

export default meta;
type Story = StoryObj<typeof meta>;

const COURSES = new Map<string, Course>([
  ["course-1", { id: "course-1", version: 1, code: "MATH-101", name: "Calculus", primary_teacher_id: "teacher-1" }],
  ["course-2", { id: "course-2", version: 2, code: "PHYS-201", name: "Waves", primary_teacher_id: "teacher-2" }],
  ["course-3", { id: "course-3", version: 1, code: "CHEM-110", name: "Molecules", primary_teacher_id: "teacher-2" }],
]);

const TEACHERS = new Map<string, User>([
  ["teacher-1", { id: "teacher-1", username: "Teacher One", role: "Teacher" }],
  ["teacher-2", { id: "teacher-2", username: "Teacher Two", role: "Teacher" }],
  ["teacher-3", { id: "teacher-3", username: "Teacher Three", role: "Teacher" }],
]);

const ROOMS = new Map<string, Room>([
  ["room-1", { id: "room-1", name: "Room 101", capacity: 20 }],
  ["room-2", { id: "room-2", name: "Room 102", capacity: 30 }],
  ["room-3", { id: "room-3", name: "Room 103", capacity: 25 }],
]);

function preflight(overrides: Partial<UsePreflightReturn>): UsePreflightReturn {
  return {
    status: "idle",
    loading: false,
    details: null,
    warnings: [],
    error: null,
    occurrencesPlanned: null,
    lastParams: null,
    check: async () => {},
    reset: () => {},
    ...overrides,
  };
}

type ConflictItem = Exclude<ConflictDetails["conflicts"], null>[number];

function conflict(session: { id: string; course: string; teacher: string; room: string | null; start: string; end: string }): ConflictItem {
  return {
    session_id: session.id,
    course_id: session.course,
    teacher_id: session.teacher,
    room_id: session.room,
    start_at: session.start,
    end_at: session.end,
  };
}

const REQUIRED = {
  course_id: "course-1",
  teacher_id: "teacher-1",
  room_id: "room-1",
  start_at: "2026-06-01T02:00:00Z",
  end_at: "2026-06-01T04:00:00Z",
};

/** Blocked with a single clash — the most common case, and auto-expanded. */
const ONE_CLASH: ConflictDetails = {
  kind: "room_overlap",
  requested: REQUIRED,
  conflicts: [conflict({ id: "s-other", course: "course-2", teacher: "teacher-2", room: "room-1", start: "2026-06-01T02:00:00Z", end: "2026-06-01T04:00:00Z" })],
  total_conflicts: 1,
};

/** Blocked with many clashes — shown collapsed with a count + expand toggle. */
const MANY_CLASHES: ConflictDetails = {
  kind: "teacher_overlap",
  requested: REQUIRED,
  conflicts: [
    conflict({ id: "s1", course: "course-2", teacher: "teacher-1", room: "room-2", start: "2026-06-01T02:00:00Z", end: "2026-06-01T03:00:00Z" }),
    conflict({ id: "s2", course: "course-3", teacher: "teacher-1", room: "room-3", start: "2026-06-01T03:00:00Z", end: "2026-06-01T04:00:00Z" }),
    conflict({ id: "s3", course: "course-2", teacher: "teacher-1", room: "room-2", start: "2026-06-02T02:00:00Z", end: "2026-06-02T04:00:00Z" }),
    conflict({ id: "s4", course: "course-3", teacher: "teacher-1", room: "room-3", start: "2026-06-03T02:00:00Z", end: "2026-06-03T04:00:00Z" }),
    conflict({ id: "s5", course: "course-2", teacher: "teacher-1", room: "room-2", start: "2026-06-04T02:00:00Z", end: "2026-06-04T04:00:00Z" }),
  ],
  total_conflicts: 5,
};

/** Student clash — the affected-student section is always auto-expanded. */
const STUDENT_CLASH: ConflictDetails = {
  kind: "student_overlap",
  requested: REQUIRED,
  conflicts: [
    conflict({ id: "s-other", course: "course-3", teacher: "teacher-2", room: "room-1", start: "2026-06-01T02:00:00Z", end: "2026-06-01T04:00:00Z" }),
  ],
  total_conflicts: 1,
  conflicting_students: [
    { student_id: "st1", full_name: "Malee Srisuk", status: "confirmed" },
    { student_id: "st2", full_name: "Anan Kitti", status: "confirmed" },
    { student_id: "st3", full_name: "Ploy Chan", status: "draft" },
  ],
};

const INSTRUCTOR = {
  kind: "teacher_availability",
  requested: REQUIRED,
  conflicts: [],
  total_conflicts: 0,
};

const ROOM_UNAVAILABLE = {
  kind: "room_availability",
  requested: REQUIRED,
  conflicts: [],
  total_conflicts: 0,
};

const NOT_ASSIGNED = {
  kind: "teacher_not_assigned_to_course",
  requested: REQUIRED,
  conflicts: null,
};

// ---------------------------------------------------------------------------
// Calm states
// ---------------------------------------------------------------------------

export const IdleMissingFields: Story = {
  args: {
    preflight: preflight({}),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
    missingFields: ["Date", "Classroom"],
  },
};

export const IdleCompleteForm: Story = {
  args: {
    preflight: preflight({}),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
    missingFields: [],
  },
};

export const Checking: Story = {
  args: {
    preflight: preflight({ status: "checking", loading: true }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
    missingFields: [],
  },
};

export const Available: Story = {
  args: {
    preflight: preflight({ status: "available" }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
    missingFields: [],
  },
};

export const ProvisionalNoRoom: Story = {
  name: "Provisional — no classroom",
  args: {
    preflight: preflight({ status: "provisional" }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
    roomMissing: true,
  },
};

export const ProvisionalUnconfirmed: Story = {
  name: "Provisional — unconfirmed details",
  args: {
    preflight: preflight({ status: "provisional" }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
    roomMissing: false,
  },
};

export const Error: Story = {
  args: {
    preflight: preflight({ status: "error" }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
  },
};

// ---------------------------------------------------------------------------
// Blocked states — the conflict reason is named up front
// ---------------------------------------------------------------------------

export const BlockedRoomOverlapSingle: Story = {
  name: "Blocked — room overlap (1 clash, auto-expanded)",
  args: {
    preflight: preflight({ status: "blocked", details: ONE_CLASH }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
  },
};

export const BlockedTeacherOverlapMany: Story = {
  name: "Blocked — teacher overlap (5 clashes, collapsed)",
  args: {
    preflight: preflight({ status: "blocked", details: MANY_CLASHES }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
  },
};

export const BlockedStudentOverlap: Story = {
  name: "Blocked — student overlap (affected students listed)",
  args: {
    preflight: preflight({ status: "blocked", details: STUDENT_CLASH }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
  },
};

export const BlockedTeacherUnavailable: Story = {
  name: "Blocked — teacher unavailable (no clash list)",
  args: {
    preflight: preflight({ status: "blocked", details: INSTRUCTOR }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
  },
};

export const BlockedRoomUnavailable: Story = {
  name: "Blocked — room unavailable (no clash list)",
  args: {
    preflight: preflight({ status: "blocked", details: ROOM_UNAVAILABLE }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
  },
};

export const BlockedTeacherNotAssigned: Story = {
  name: "Blocked — teacher not assigned to course",
  args: {
    preflight: preflight({ status: "blocked", details: NOT_ASSIGNED }),
    coursesById: COURSES,
    teachersById: TEACHERS,
    roomsById: ROOMS,
  },
};
