import { format } from 'date-fns';
import { Link } from 'react-router-dom';
import { ExternalLink } from 'lucide-react';
import type { TeacherDashboardSession, TeacherDashboardSitInVisitor, AbsentStudent } from '../../types';

type CourseGroupInfo = {
  subjectName: string | null;
  courseCode: string;
  courseName: string;
  courseId: string;
  sessions: TeacherDashboardSession[];
  scheduleSummary: string;
};

type CourseDashboardCardProps = {
  course: CourseGroupInfo;
};

function dedupAbsences(absences: AbsentStudent[]): AbsentStudent[] {
  const seen = new Set<string>();
  return absences.filter((a) => {
    if (seen.has(a.absence_id)) return false;
    seen.add(a.absence_id);
    return true;
  });
}

function relativeTime(dateStr: string): string {
  const d = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - d.getTime();
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffDays === 0) return `Today ${format(d, 'HH:mm')}`;
  if (diffDays === 1) return 'Yesterday';
  if (diffDays < 7 && now.getDay() >= d.getDay()) return format(d, 'EEE');
  return format(d, 'd MMM');
}

function dedupSitIns(visitors: TeacherDashboardSitInVisitor[]): TeacherDashboardSitInVisitor[] {
  const seen = new Set<string>();
  return visitors.filter((v) => {
    const key = `${v.absence_id}-${v.wcode}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export default function CourseDashboardCard({ course }: CourseDashboardCardProps) {
  const allAbsences = dedupAbsences(course.sessions.flatMap((s) => s.absent_students ?? []));
  const allSitIns = dedupSitIns(course.sessions.flatMap((s) => s.sit_in_visitors ?? []));
  const hasAbsences = allAbsences.length > 0;
  const hasSitIns = allSitIns.length > 0;

  const displayName = course.subjectName ?? course.courseName;

  return (
    <div className="rounded border border-gray-200 bg-white">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-50">
        <div className="min-w-0">
          <p className="text-[14px] font-bold text-gray-900 truncate">{displayName}</p>
          <p className="text-[12px] text-gray-500 truncate">
            {course.courseCode}{course.scheduleSummary ? ` · ${course.scheduleSummary}` : null}
          </p>
        </div>
      </div>

      {/* Absence list */}
      {hasAbsences ? (
        <div className="px-4 py-2.5 border-b border-gray-50">
          <p className="mb-1 flex items-center gap-1.5 text-[12px] text-red-600">
            <span className="h-2 w-2 rounded-full bg-red-500 shrink-0" />
            {allAbsences.length} {allAbsences.length === 1 ? 'absence' : 'absences'} this week
          </p>
          <div className="divide-y divide-gray-50">
            {allAbsences.slice(0, 8).map((s) => (
              <div key={s.absence_id} className="flex items-center justify-between py-1">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="truncate text-[13px] text-gray-800">
                    {s.nickname ?? s.student_name ?? s.wcode}
                  </span>
                  <span className="shrink-0 text-[11px] text-gray-400">({s.wcode})</span>
                  {s.created_at ? (
                    <span className="shrink-0 text-[11px] text-gray-400">{relativeTime(s.created_at)}</span>
                  ) : null}
                </div>
                <Link
                  to={`/absences/${s.absence_id}`}
                  className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
                >
                  View <ExternalLink className="inline h-3 w-3" />
                </Link>
              </div>
            ))}
          </div>
          {allAbsences.length > 8 ? (
            <p className="mt-1 text-[12px] text-gray-400">…and {allAbsences.length - 8} more</p>
          ) : null}
        </div>
      ) : null}

      {/* Sit-in list */}
      {hasSitIns ? (
        <div className="px-4 py-2.5 border-b border-gray-50">
          <p className="mb-1 flex items-center gap-1.5 text-[12px] text-amber-600">
            <span className="h-2 w-2 rounded-full bg-amber-500 shrink-0" />
            {allSitIns.length} sit-in {allSitIns.length === 1 ? 'visitor' : 'visitors'} this week
          </p>
          <div className="divide-y divide-gray-50">
            {allSitIns.map((v) => {
              const absentLabel = v.absent_subject_name ?? v.from_course_code;
              const timeStr = v.session_start_at
                ? `${format(new Date(v.session_start_at), 'EEE HH:mm')}–${format(new Date(v.session_end_at), 'HH:mm')}`
                : null;
              return (
                <div key={`${v.absence_id}-${v.wcode}`} className="py-1">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-1.5 min-w-0">
                      <span className="truncate text-[13px] text-gray-800">
                        {v.nickname ?? v.student_name ?? v.wcode}
                      </span>
                      <span className="shrink-0 text-[11px] text-amber-600">
                        absent from {absentLabel}
                      </span>
                    </div>
                    <Link
                      to={`/absences/${v.absence_id}`}
                      className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
                    >
                      View <ExternalLink className="inline h-3 w-3" />
                    </Link>
                  </div>
                  {timeStr ? (
                    <p className="text-[11px] text-gray-400 pl-0">{timeStr}</p>
                  ) : null}
                </div>
              );
            })}
          </div>
        </div>
      ) : null}

      {/* Empty state */}
      {!hasAbsences && !hasSitIns ? (
        <div className="px-4 py-3 text-[13px] text-gray-400">
          <span className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-green-500 shrink-0" />
            All clear — no absences or sit-ins this week
          </span>
        </div>
      ) : null}
    </div>
  );
}
