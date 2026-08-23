import type { ReactNode } from "react";
import type { Course } from "../types";
import { formatRemainingHours, remainingMinutes, remainingStatus } from "../domain/sessionUsage";
import { formatLegacySyncTime } from "../domain/legacyCourse";

function StatItem({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <span className="text-[11px] font-medium uppercase tracking-wide text-[var(--color-wi-text-light)]">{label}</span>
      <span className="text-[13px] text-[var(--color-wi-text)]">{value}</span>
    </div>
  );
}

/** Compact summary row at the top of the course detail page — the four fields
 *  the legacy site showed at a glance (Teacher, Hour, Student, Type), plus the
 *  hours remaining once the scheduled sessions are counted. */
export function CourseInfoStrip({ course, teacherName, usedMinutes = 0 }: { course: Course; teacherName?: string | null; usedMinutes?: number }) {
  const teacherValue = teacherName ?? course.teacher_name ?? (
    course.teachers?.length
      ? course.teachers.map((t) => t.full_name || t.username).join(", ")
      : null
  );

  const remaining = remainingMinutes(course.hour, usedMinutes);
  const status = remaining == null ? null : remainingStatus(remaining);
  const expiryDate = course.expires_at ? new Date(course.expires_at).toLocaleDateString() : null;
  const expiryValue = course.expiry_status === "expired"
    ? expiryDate ? `Expired on ${expiryDate}` : "Expired"
    : course.expiry_status === "active" && expiryDate
      ? `Until ${expiryDate}`
      : "No expiration";
  const legacySyncValue = course.legacy_course_id == null
    ? null
    : course.legacy_last_synced_at
      ? <time dateTime={course.legacy_last_synced_at}>{formatLegacySyncTime(course.legacy_last_synced_at)}</time>
      : "Not synced yet";

  return (
    <div
      className="mt-3 flex flex-wrap items-start gap-x-6 gap-y-2 border-t border-wi-line pt-3"
      aria-label="Course summary"
    >
      <StatItem label="Teacher" value={teacherValue ?? "—"} />
      <StatItem label="Hour" value={course.hour ?? "—"} />
      <StatItem
        label="Remaining"
        value={
          remaining == null || status == null ? (
            "—"
          ) : (
            <span
              data-testid="remaining-pill"
              data-usage={status}
              className={`inline-flex items-center rounded-full px-2 py-0.5 font-medium ${
                status === "remaining"
                  ? "bg-[var(--color-wi-green-bg)]"
                  : status === "over"
                    ? "bg-[var(--color-wi-danger-bg)]"
                    : "bg-[var(--color-wi-row-alt)]"
              }`}
            >
              {formatRemainingHours(remaining)}
            </span>
          )
        }
      />
      <StatItem label="Student" value={course.student_count ?? "—"} />
      <StatItem label="Type" value={course.course_type ?? "—"} />
      {legacySyncValue !== null && <StatItem label="Last sync" value={legacySyncValue} />}
      {(course.cycle_id || course.cycle_label || course.expiry_days != null || course.expiry_status) && <StatItem label="Cycle" value={course.cycle_label ?? "—"} />}
      {(course.expiry_days != null || course.expiry_status) && <StatItem label="Expires" value={expiryValue} />}
    </div>
  );
}
