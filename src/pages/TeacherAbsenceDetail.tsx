import { useCallback, useEffect, useState } from "react";
import { ArrowLeft, CalendarDays, Eye, MapPin } from "lucide-react";
import { Link, useParams } from "react-router-dom";
import { apiJson } from "../api/client";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import Button from "../components/ui/Button";
import { useAuth } from "../hooks/useAuth";
import { useToast } from "../hooks/useToast";
import type { TeacherAbsenceDetail as TeacherAbsenceDetailData, TeacherAbsenceSession } from "../types";
import OverrideSitInModal from "../components/absences/OverrideSitInModal";
import { formatUTCToZone, formatZoneDateKey } from "../utils/timezone";

const INSTITUTE_TIME_ZONE = "Asia/Bangkok";

function formatDate(value: string): string {
  return formatZoneDateKey(value, INSTITUTE_TIME_ZONE, "d MMM yyyy") ?? value;
}

function formatSession(value: string): string {
  return formatUTCToZone(value, INSTITUTE_TIME_ZONE, "d MMM yyyy, HH:mm") ?? value;
}

function titleCase(value: string): string {
  return value.replace(/_/g, " ").replace(/^./, (letter) => letter.toUpperCase());
}

function SessionList({ title, sessions }: { title: string; sessions: TeacherAbsenceSession[] }) {
  return (
    <section className="rounded-sm border var(--color-wi-line) bg-white p-5">
      <h2 className="text-sm font-semibold text-[var(--color-wi-text)]">{title}</h2>
      {sessions.length === 0 ? (
        <p className="mt-3 text-sm text-[var(--color-wi-text-light)]">No sessions assigned to you.</p>
      ) : (
        <ul className="mt-3 divide-y divide-wi-line">
          {sessions.map((session) => (
            <li key={session.session_id} className="py-3 first:pt-0 last:pb-0">
              <p className="font-medium text-[var(--color-wi-text)]">{session.subject_name?.trim() || session.course_name}</p>
              <p className="mt-1 flex items-center gap-1.5 text-sm text-[var(--color-wi-text-light)]">
                <CalendarDays className="h-4 w-4" aria-hidden="true" />
                {formatSession(session.start_at)}
              </p>
              {session.room_name ? (
                <p className="mt-1 flex items-center gap-1.5 text-sm text-[var(--color-wi-text-light)]">
                  <MapPin className="h-4 w-4" aria-hidden="true" />{session.room_name}
                </p>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

export default function TeacherAbsenceDetail() {
  const { id = "" } = useParams();
  const { user } = useAuth();
  const { addToast } = useToast();
  const backTo = user?.role === 'Admin' ? '/absences/dashboard' : '/teacher-dashboard';
  const canOverrideSitIn = user?.role === 'Admin';
  const [detail, setDetail] = useState<TeacherAbsenceDetailData | null>(null);
  const [loading, setLoading] = useState(true);
  const [overrideOpen, setOverrideOpen] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setDetail(await apiJson<TeacherAbsenceDetailData>(`/api/v1/teacher/absences/${id}`, { method: "GET" }));
    } catch (error) {
      setDetail(null);
      addToast("error", error instanceof Error ? error.message : "Failed to load absence");
    } finally {
      setLoading(false);
    }
  }, [addToast, id]);

  useEffect(() => { void load(); }, [load]);

  if (loading) return <LoadingSkeleton lines={6} />;
  if (!detail) {
    return (
      <div className="rounded-sm border var(--color-wi-line) bg-white p-6">
        <h1 className="text-lg font-semibold text-[var(--color-wi-text)]">Absence not available</h1>
        <p className="mt-2 text-sm text-[var(--color-wi-text-light)]">This request does not exist or is not assigned to one of your courses.</p>
        <Link to={backTo} className="mt-4 inline-flex text-sm font-medium text-[var(--color-wi-primary)]">Back to dashboard</Link>
      </div>
    );
  }

  const displayName = detail.student_nickname ?? detail.student_name ?? detail.wcode;
  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <Link to={backTo} className="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-wi-primary)] hover:underline">
        <ArrowLeft className="h-4 w-4" aria-hidden="true" /> Back to dashboard
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm text-[var(--color-wi-text-light)]">Student absence request</p>
          <h1 className="mt-1 text-2xl font-semibold text-gray-950">{displayName}</h1>
          <p className="mt-1 font-mono text-sm text-[var(--color-wi-text-light)]">{detail.wcode}</p>
        </div>
        <div className="flex items-center gap-2">
          <span className="rounded-full bg-[var(--color-wi-row-alt)] px-3 py-1 text-xs font-semibold text-[var(--color-wi-text-light)]">{titleCase(detail.status)}</span>
          <span className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-3 py-1 text-xs font-semibold text-blue-700">
            <Eye className="h-3.5 w-3.5" aria-hidden="true" /> Read-only
          </span>
          {canOverrideSitIn ? (
            <Button size="sm" variant="secondary" onClick={() => setOverrideOpen(true)}>Override Sit-in</Button>
          ) : null}
        </div>
      </header>

      <section className="grid gap-4 rounded-sm border var(--color-wi-line) bg-white p-5 sm:grid-cols-2">
        <div><p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Subject</p><p className="mt-1 font-medium text-[var(--color-wi-text)]">{detail.subject_name?.trim() || detail.course_name}</p></div>
        <div><p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Absence dates</p><p className="mt-1 font-medium text-[var(--color-wi-text)]">{formatDate(detail.date_from)}{detail.date_from === detail.date_to ? "" : ` – ${formatDate(detail.date_to)}`}</p></div>
        <div><p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Reason category</p><p className="mt-1 text-[var(--color-wi-text)]">{detail.reason_category ? titleCase(detail.reason_category) : "Not provided"}</p></div>
        <div><p className="text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Reason</p><p className="mt-1 whitespace-pre-wrap text-[var(--color-wi-text)]">{detail.reason ?? "Not provided"}</p></div>
      </section>

      <div className="grid gap-5 lg:grid-cols-2">
        <SessionList title="Missed sessions assigned to you" sessions={detail.missed_sessions} />
        <SessionList title="Sit-in sessions assigned to you" sessions={detail.sit_in_sessions} />
      </div>

      {overrideOpen && canOverrideSitIn ? (
        <OverrideSitInModal
          absenceId={detail.id}
          version={detail.version}
          currentMethod={detail.sit_in_method}
          currentCourseId={detail.sit_in_course_id}
          onClose={() => setOverrideOpen(false)}
          onSaved={() => void load()}
        />
      ) : null}
    </div>
  );
}
