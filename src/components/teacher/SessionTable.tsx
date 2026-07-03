import { useState, Fragment } from 'react';
import { Link } from 'react-router-dom';
import { CalendarX } from 'lucide-react';
import type { TeacherDashboardSession, AbsentStudent, TeacherDashboardSitInVisitor } from '../../types';
import EmptyState from '../ui/EmptyState';
import { formatUTCToZone, formatZoneDateKey, utcISOToZoneDate } from '../../utils/timezone';

type SessionTableProps = {
  sessions: TeacherDashboardSession[];
  zone: string;
};

function formatDate(iso: string, zone: string): string {
  return formatUTCToZone(iso, zone, 'EEE, d MMM') ?? iso;
}

function formatHour(iso: string, zone: string): string {
  return formatUTCToZone(iso, zone, 'HH:mm') ?? '--:--';
}

function initials(name: string): string {
  return name.split(' ').map((part) => part.charAt(0)).join('').toUpperCase().slice(0, 2);
}

function AbsenceCard({ student, sessionCourse, sessionStart, sessionEnd, zone }: {
  student: AbsentStudent;
  sessionCourse: string;
  sessionStart: string;
  sessionEnd: string;
  zone: string;
}) {
  const name = student.nickname ?? student.student_name ?? student.wcode;
  return (
    <div className="flex items-center gap-3 rounded-sm border border-gray-100 bg-gray-50/50 px-3 py-2.5">
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-[10px] font-bold text-white">
        {initials(name)}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="text-[13px] font-medium text-gray-900">{name}</span>
          <span className="font-mono text-[11px] text-gray-500">{student.wcode}</span>
        </div>
        <p className="text-[11px] text-gray-500">
          Absent from {sessionCourse} · {formatDate(sessionStart, zone)}, {formatHour(sessionStart, zone)}–{formatHour(sessionEnd, zone)}
        </p>
      </div>
      <Link
        to={`/teacher-dashboard/absences/${student.absence_id}`}
        className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
      >
        View →
      </Link>
    </div>
  );
}

function SitInCard({ visitor, zone }: { visitor: TeacherDashboardSitInVisitor; zone: string }) {
  const name = visitor.nickname ?? visitor.student_name ?? visitor.wcode;
  return (
    <div className="flex items-center gap-3 rounded-sm border border-gray-100 bg-gray-50/50 px-3 py-2.5">
      <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-wi-primary)] text-[10px] font-bold text-white">
        {initials(name)}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="text-[13px] font-medium text-gray-900">{name}</span>
          <span className="font-mono text-[11px] text-gray-500">{visitor.wcode}</span>
        </div>
        <p className="text-[11px] text-gray-500">
          Sit-in from {visitor.from_course_code} · {formatDate(visitor.session_start_at, zone)}, {formatHour(visitor.session_start_at, zone)}–{formatHour(visitor.session_end_at, zone)}
        </p>
      </div>
      <Link
        to={`/teacher-dashboard/absences/${visitor.absence_id}`}
        className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
      >
        View →
      </Link>
    </div>
  );
}

export default function SessionTable({ sessions, zone }: SessionTableProps) {
  const [expandedId, setExpandedId] = useState<string | null>(null);

  if (sessions.length === 0) {
    return (
      <EmptyState
        message="No sessions in this period."
        icon={<CalendarX className="h-10 w-10" />}
      />
    );
  }

  const sorted = [...sessions].sort(
    (a, b) => new Date(a.start_at).getTime() - new Date(b.start_at).getTime(),
  );

  const grouped = new Map<string, TeacherDashboardSession[]>();
  for (const s of sorted) {
    const key = utcISOToZoneDate(s.start_at, zone);
    if (!key) continue;
    const group = grouped.get(key);
    if (group) {
      group.push(s);
    } else {
      grouped.set(key, [s]);
    }
  }

  return (
    <div className="overflow-x-auto rounded-sm border border-gray-200 bg-white">
      <table className="w-full text-sm">
        <thead className="text-left text-xs font-semibold uppercase tracking-wide text-gray-500">
          <tr>
            <th className="w-[32px] px-2 py-2" />
            <th className="px-3 py-2 w-[120px]">Time</th>
            <th className="px-3 py-2">Course / Subject</th>
            <th className="px-3 py-2 w-[120px]">Room</th>
            <th className="px-3 py-2 w-[100px]">Absences</th>
            <th className="px-3 py-2 w-[100px]">Sit-ins</th>
            <th className="px-3 py-2 w-[90px]">Status</th>
          </tr>
        </thead>
        <tbody>
          {Array.from(grouped.entries()).map(([dateKey, groupSessions]) => (
            <Fragment key={dateKey}>
              <tr className="bg-gray-50">
                <td colSpan={7} className="px-3 py-2 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
                  {formatZoneDateKey(dateKey, zone, 'EEE, d MMM') ?? dateKey}
                </td>
              </tr>
              {groupSessions.map((s) => {
                const isExpanded = expandedId === s.id;
                const hasAbsences = (s.absent_students?.length ?? 0) > 0;
                const hasSitIns = (s.sit_in_visitors?.length ?? 0) > 0;

                function toggle() {
                  setExpandedId(isExpanded ? null : s.id);
                }

                return (
                  <Fragment key={s.id}>
                    <tr
                      className="group align-middle hover:bg-blue-50/40 cursor-pointer"
                      onClick={toggle}
                    >
                      <td className="w-[32px] px-2 py-3 text-center text-gray-400">
                        <span className={`inline-block transition-transform ${isExpanded ? 'rotate-90' : ''}`}>
                          ▸
                        </span>
                      </td>
                      <td className="px-3 py-3 whitespace-nowrap text-[13px] text-gray-900">
                        {formatHour(s.start_at, zone)} – {formatHour(s.end_at, zone)}
                      </td>
                      <td className="px-3 py-3">
                        <div className="max-w-[200px] truncate font-medium text-gray-900" title={`${s.course_code} — ${s.course_name}`}>
                          {s.course_code}
                        </div>
                        {s.subject_name ? (
                          <div className="text-xs text-gray-500">{s.subject_name}</div>
                        ) : null}
                      </td>
                      <td className="px-3 py-3 whitespace-nowrap text-sm text-gray-700">
                        {s.room_name ?? <span className="text-gray-300">—</span>}
                      </td>
                      <td className="px-3 py-3 whitespace-nowrap">
                        {hasAbsences ? (
                          <span className="inline-flex items-center gap-1 rounded-full border border-red-200 bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700">
                            <span className="h-1.5 w-1.5 rounded-full bg-red-500" />
                            {s.absent_students!.length}
                          </span>
                        ) : (
                          <span className="text-xs text-gray-300">—</span>
                        )}
                      </td>
                      <td className="px-3 py-3 whitespace-nowrap">
                        {hasSitIns ? (
                          <span className="inline-flex items-center gap-1 rounded-full border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
                            <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
                            {s.sit_in_visitors!.length}
                          </span>
                        ) : (
                          <span className="text-xs text-gray-300">—</span>
                        )}
                      </td>
                      <td className="px-3 py-3 whitespace-nowrap">
                        {hasAbsences ? (
                          <span className="inline-flex items-center rounded-full border border-red-200 bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700">
                            Absences
                          </span>
                        ) : hasSitIns ? (
                          <span className="inline-flex items-center rounded-full border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
                            Sit-ins
                          </span>
                        ) : (
                          <span className="inline-flex items-center rounded-full border border-green-200 bg-green-50 px-2 py-0.5 text-xs font-medium text-green-700">
                            OK
                          </span>
                        )}
                      </td>
                    </tr>

                    {isExpanded ? (
                      <tr>
                        <td colSpan={7} className="border-b border-gray-100 bg-gray-50/30 px-6 py-4">
                          <div className="space-y-4">
                            {hasAbsences ? (
                              <div>
                                <h5 className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-red-700">
                                  <span className="h-2 w-2 rounded-full bg-red-500" />
                                  Absences
                                  <span className="ml-1 inline-flex min-w-5 items-center justify-center rounded-full bg-red-100 px-1.5 py-0.5 text-[10px] font-semibold text-red-700">
                                    {s.absent_students!.length}
                                  </span>
                                </h5>
                                <div className="space-y-1.5">
                                  {s.absent_students!.map((st) => (
                                    <AbsenceCard
                                      key={st.wcode}
                                      student={st}
                                      sessionCourse={s.course_code}
                                      sessionStart={s.start_at}
                                      sessionEnd={s.end_at}
                                      zone={zone}
                                    />
                                  ))}
                                </div>
                              </div>
                            ) : null}

                            {hasSitIns ? (
                              <div>
                                <h5 className="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-amber-700">
                                  <span className="h-2 w-2 rounded-full bg-amber-500" />
                                  Sit-ins
                                  <span className="ml-1 inline-flex min-w-5 items-center justify-center rounded-full bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700">
                                    {s.sit_in_visitors!.length}
                                  </span>
                                </h5>
                                <div className="space-y-1.5">
                                  {s.sit_in_visitors!.map((v) => (
                                    <SitInCard key={v.wcode} visitor={v} zone={zone} />
                                  ))}
                                </div>
                              </div>
                            ) : null}
                          </div>
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              })}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}
