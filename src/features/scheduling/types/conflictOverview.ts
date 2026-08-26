import { z } from "zod";

const conflictSessionSchema = z.object({
  session_id: z.string(),
  course_id: z.string(),
  course_code: z.string(),
  course_name: z.string(),
  subject_id: z.string(),
  subject_name: z.string(),
  teacher_id: z.string(),
  teacher_name: z.string(),
  room_id: z.string().nullable(),
  room_name: z.string().nullable(),
  start_at: z.string(),
  end_at: z.string(),
});

export const conflictOverviewResponseSchema = z.object({
  items: z.array(
    z.object({
      id: z.string(),
      conflict_type: z.enum(["room_overlap", "teacher_overlap", "student_overlap"]),
      primary_session: conflictSessionSchema,
      conflicting_sessions: z.array(conflictSessionSchema),
      affected_students: z.array(
        z.object({
          student_id: z.string(),
          wcode: z.string(),
          full_name: z.string(),
        }),
      ),
      shared_resource: z.object({
        type: z.enum(["room", "teacher", "student"]),
        id: z.string(),
        name: z.string(),
      }),
      detected_at: z.string(),
    }),
  ),
  limit: z.number().int().positive(),
  has_next: z.boolean(),
  has_prev: z.boolean(),
  next_cursor: z.string().nullable(),
  prev_cursor: z.string().nullable(),
});

export const conflictSummarySchema = z.object({
  total_conflicts: z.number().int().nonnegative(),
  room_overlaps: z.number().int().nonnegative(),
  teacher_overlaps: z.number().int().nonnegative(),
  student_overlaps: z.number().int().nonnegative(),
});

export type ConflictOverviewResponse = z.infer<typeof conflictOverviewResponseSchema>;
export type ConflictSummaryResponse = z.infer<typeof conflictSummarySchema>;
export type ConflictOverviewItem = ConflictOverviewResponse["items"][number];
export type ConflictSession = ConflictOverviewItem["primary_session"];
export type ConflictType = ConflictOverviewItem["conflict_type"];
