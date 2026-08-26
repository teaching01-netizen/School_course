import { Link } from "react-router-dom";
import type { ConflictOverviewItem } from "@/features/scheduling/types/conflictOverview";

export function ConflictDetailPanel({ conflict }: Readonly<{ conflict: ConflictOverviewItem }>) {
  return (
    <div className="grid gap-3 bg-[var(--color-wi-row-alt)] px-10 py-4 md:grid-cols-2" id={`conflict-details-${conflict.id}`}>
      <SessionDetail title="Primary session" session={conflict.primary_session} />
      {conflict.conflicting_sessions.map((session) => <SessionDetail key={session.session_id} title="Conflicting session" session={session} />)}
      {conflict.affected_students.length > 0 ? (
        <div className="md:col-span-2">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Affected students</h3>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {conflict.affected_students.map((student) => <Link key={student.student_id} to={`/courses/${conflict.primary_session.course_id}`} className="rounded-sm border border-blue-200 bg-blue-50 px-2 py-1 text-xs text-blue-800 hover:underline">{student.full_name} · {student.wcode}</Link>)}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function SessionDetail({ title, session }: Readonly<{ title: string; session: ConflictOverviewItem["primary_session"] }>) {
  return (
    <div className="rounded-sm border border-wi-line bg-white p-3">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">{title}</h3>
      <Link to={`/courses/${session.course_id}`} className="mt-2 inline-block text-sm font-semibold text-[var(--color-wi-primary)] hover:underline">{session.subject_name}</Link>
      <p className="mt-0.5 text-xs text-[var(--color-wi-text-light)]"><Link to={`/courses/${session.course_id}`} className="hover:text-[var(--color-wi-primary)] hover:underline">{session.course_code} · {session.course_name}</Link></p>
      <dl className="mt-3 grid grid-cols-[5rem_1fr] gap-x-2 gap-y-1 text-xs">
        <dt className="text-[var(--color-wi-text-light)]">Teacher</dt><dd><Link to={`/courses/${session.course_id}`} className="text-[var(--color-wi-primary)] hover:underline">{session.teacher_name}</Link></dd>
        <dt className="text-[var(--color-wi-text-light)]">Room</dt><dd>{session.room_name ?? "Unassigned"}</dd>
        <dt className="text-[var(--color-wi-text-light)]">Time</dt><dd>{formatDateTimeRange(session.start_at, session.end_at)}</dd>
      </dl>
    </div>
  );
}

export function formatDateTimeRange(startAt: string, endAt: string): string {
  const start = new Date(startAt);
  const end = new Date(endAt);
  const date = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(start);
  const time = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
  return `${date}, ${time.format(start)}–${time.format(end)}`;
}
