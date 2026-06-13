export type CRMConflictCourse = {
  id?: string;
  subject_name?: string;
  code?: string;
  name?: string;
};

export type CRMStudentScheduleConflictDetails = {
  kind: "crm_student_schedule_conflict";
  student?: {
    wcode?: string;
    full_name?: string;
  };
  target_course?: CRMConflictCourse;
  conflicts?: Array<{
    course?: CRMConflictCourse;
    start_at?: string;
    end_at?: string;
  }>;
};

export function crmCourseLabel(course?: CRMConflictCourse | null): string | null {
  if (!course) return null;
  return course.subject_name || course.name || course.code || null;
}

export function formatCRMConflictTime(startAt?: string, endAt?: string): string | null {
  if (!startAt || !endAt) return null;
  const start = new Date(startAt);
  const end = new Date(endAt);
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return null;
  const date = start.toLocaleDateString("en-GB", { day: "numeric", month: "short", timeZone: "Asia/Bangkok" });
  const startTime = start.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", timeZone: "Asia/Bangkok" });
  const endTime = end.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", timeZone: "Asia/Bangkok" });
  return `${date}, ${startTime}-${endTime}`;
}

export function getCRMConflictDetails<T extends { details?: unknown }>(details?: T["details"]): CRMStudentScheduleConflictDetails | null {
  if (!details || typeof details !== "object") return null;
  if ("kind" in details && details.kind === "crm_student_schedule_conflict") {
    return details as CRMStudentScheduleConflictDetails;
  }
  if ("details" in details && details.details && typeof details.details === "object") {
    const nested = details.details as Record<string, unknown>;
    if (nested.kind === "crm_student_schedule_conflict") {
      return nested as CRMStudentScheduleConflictDetails;
    }
  }
  return null;
}

export function formatCRMConflictTechnicalDetail(
  message: string | undefined,
  studentName: string | null | undefined,
  studentWCode: string | null | undefined,
  targetCourse: string | null,
  conflictingCourse: string | null,
  conflictTime: string | null,
): string {
  if (!studentName && !studentWCode && !targetCourse && !conflictingCourse && !conflictTime) {
    return message ?? "Student schedule conflict";
  }
  const student = `${studentName || "Student"}${studentWCode ? ` (${studentWCode})` : ""}`;
  const target = targetCourse ? ` to ${targetCourse}` : "";
  const conflict = conflictingCourse || conflictTime
    ? ` because they already have ${conflictingCourse ?? "another course"}${conflictTime ? ` at ${conflictTime}` : ""}`
    : "";
  return `Student schedule conflict: ${student} cannot be added${target}${conflict}`;
}
