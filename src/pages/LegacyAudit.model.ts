export type LegacyAuditTotals = {
  linked_courses: number;
  archived_courses: number;
  synced_courses: number;
  legacy_sessions: number;
  active_sessions: number;
  soft_deleted_sessions: number;
  external_series: number;
  students_imported: number;
  mapped_rooms: number;
  mapped_teachers: number;
  mapped_subjects: number;
};

export type LegacyAuditRuns = {
  completed_runs: number;
  entities_parsed: number;
  entities_applied: number;
  parse_failures: number;
  reconciliation_mismatches: number;
  last_successful_at: string | null;
};

export type LegacyAuditBucket = {
  cause: "open_conflict" | "closed_conflict" | "dead_letter" | "partial_snapshot";
  entity_type: string;
  key: string;
  count: number;
};

export type LegacyAuditSkips = {
  sessions_skipped_total: number;
  sessions_skipped_open: number;
  courses_skipped_total: number;
  courses_skipped_open: number;
  partial_snapshots: number;
  by_cause: LegacyAuditBucket[];
};

export type SkippedSession = {
  legacy_schedule_id: string;
  date: string | null;
  begin: string | null;
  end: string | null;
  classroom: string | null;
  conflict_type: string;
  category: string;
  message: string | null;
  status: string;
  created_at: string | null;
  course_id: string | null;
  course_code: string | null;
  course_name: string | null;
  legacy_course_id: string;
};

export type SkippedCourse = {
  reason_kind: "conflict" | "dead_letter";
  external_id: string;
  conflict_type: string;
  error_category: string | null;
  message: string | null;
  status: string;
  created_at: string | null;
  course_id: string | null;
  course_code: string | null;
  course_name: string | null;
};

export type DeadLetter = {
  id: string;
  job_type: string;
  entity_type: string | null;
  external_id: string | null;
  error_category: string | null;
  last_error: string;
  attempts: number;
  created_at: string | null;
};

export type LegacyAudit = {
  generated_at: string;
  totals: LegacyAuditTotals;
  runs: LegacyAuditRuns;
  skips: LegacyAuditSkips;
  skipped_sessions: SkippedSession[];
  skipped_courses: SkippedCourse[];
  dead_letters: DeadLetter[];
};

export function formatTime(value: string | null): string {
  if (!value) return "Not available";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Not available";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
}

export function formatCount(value: number): string {
  return new Intl.NumberFormat(undefined).format(value);
}

export const causeCopy: Record<LegacyAuditBucket["cause"], string> = {
  open_conflict: "Open conflicts",
  closed_conflict: "Resolved/ignored conflicts",
  dead_letter: "Dead letters",
  partial_snapshot: "Partial snapshots",
};

export function conflictTypeCopy(type: string): string {
  switch (type) {
    case "room_overlap":
      return "Room overlap";
    case "teacher_overlap":
      return "Teacher overlap";
    case "availability":
      return "Teacher unavailable";
    case "schedule_exclusion":
      return "Schedule exclusion";
    case "code_claimed":
      return "Course code already linked";
    default:
      return type;
  }
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "The audit data could not be loaded.";
}
