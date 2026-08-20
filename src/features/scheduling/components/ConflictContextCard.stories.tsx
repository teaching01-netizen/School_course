import type { Meta, StoryObj } from "@storybook/react";
import { ConflictContextCard, parseConflictContext } from "./ConflictContextCard";
import type { Course } from "@/features/courses/types";

/**
 * The reason-first card shown at the top of Slot Finder when the user arrives
 * via "Find alternative slots →" from a blocked availability strip. It restates
 * the conflict (naming the actual room / teacher / students), the requested
 * window, and the suggested next step.
 */
const meta = {
  title: "Scheduling/ConflictContextCard",
  component: ConflictContextCard,
  parameters: {
    layout: "centered",
    docs: {
      description: {
        component:
          "Carried-over conflict context for the Slot Finder landing. The blocked availability strip links here with query params; the card restates the reason so the search page never loses the context of what went wrong.",
      },
    },
  },
  decorators: [
    (Story) => (
      <div className="w-[40rem]">{Story()}</div>
    ),
  ],
} satisfies Meta<typeof ConflictContextCard>;

export default meta;
type Story = StoryObj<typeof meta>;

const COURSES = new Map<string, Course>([
  ["course-1", { id: "course-1", version: 1, code: "MATH-101", name: "Calculus", primary_teacher_id: "teacher-1" }],
]);

function contextFrom(query: string) {
  const ctx = parseConflictContext(new URLSearchParams(query));
  if (!ctx) throw new Error(`invalid story query: ${query}`);
  return ctx;
}

const BASE_QUERY = [
  "course_id=course-1",
  "teacher_id=teacher-1",
  "room_id=room-1",
  "room=Room+101",
  "teacher=Teacher+One",
  "start_at=2026-06-01T02%3A00%3A00Z",
  "end_at=2026-06-01T04%3A00%3A00Z",
].join("&");

const STUDENT_QUERY = [
  "course_id=course-1",
  "teacher_id=teacher-1",
  "start_at=2026-06-01T02%3A00%3A00Z",
  "end_at=2026-06-01T04%3A00%3A00Z",
  "student_count=3",
  "student_id=st1",
  "student=Ariya+S.",
].join("&");

export const RoomOverlap: Story = {
  name: "Room overlap — names the room",
  args: {
    context: contextFrom(`kind=room_overlap&${BASE_QUERY}`),
    coursesById: COURSES,
  },
};

export const TeacherOverlap: Story = {
  name: "Teacher overlap — names the teacher",
  args: {
    context: contextFrom(`kind=teacher_overlap&${BASE_QUERY}`),
    coursesById: COURSES,
  },
};

export const StudentOverlap: Story = {
  name: "Student overlap — counts the students",
  args: {
    context: contextFrom(`kind=student_overlap&${STUDENT_QUERY}`),
    coursesById: COURSES,
  },
};

export const TeacherNotAssigned: Story = {
  name: "Teacher not assigned to course",
  args: {
    context: contextFrom(`kind=teacher_not_assigned_to_course&${BASE_QUERY}`),
    coursesById: COURSES,
  },
};