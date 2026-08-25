import { useEffect, useMemo, useState, type FormEvent } from "react";
import { ArrowLeft, ExternalLink } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router-dom";
import ConfirmModal from "../components/ConfirmModal";
import Modal from "../components/Modal";
import Button from "../components/ui/Button";
import EmptyState from "../components/ui/EmptyState";
import Input from "../components/ui/Input";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import PageHeading from "../components/ui/PageHeading";
import { useToast } from "../hooks/useToast";
import {
	deleteCourseGroup,
	getCourseGroup,
	getCourseGroupSessions,
	getInstituteTimeMeta,
	getRooms,
	updateCourseGroup,
	type CourseGroupSessions,
} from "../features/courses/api/courseApi";
import type { CourseGroup } from "../features/courses/types";
import { fmtDuration, minutesBetween } from "../features/scheduling/domain/time";
import { formatUTCToZone, utcISOToZoneDate } from "../utils/timezone";

const DEFAULT_TIME_ZONE = "Asia/Bangkok";

function displayTeacherName(fullName: string | null, username: string): string {
  return fullName?.trim() || username;
}

function displayDate(value: string, zone: string): string {
  return formatUTCToZone(value, zone, "EEE d MMM yy") ?? value;
}

function displayTime(value: string, zone: string): string {
  return formatUTCToZone(value, zone, "HH:mm") ?? value.slice(11, 16);
}

function mergedTimeByDate(sessions: CourseGroupSessions[], zone: string): Map<string, { start_at: string; end_at: string }> {
  const ranges = new Map<string, { start_at: string; end_at: string }>();
  for (const session of sessions) {
    const date = utcISOToZoneDate(session.start_at, zone);
    if (!date) continue;
    const current = ranges.get(date);
    if (!current) {
      ranges.set(date, { start_at: session.start_at, end_at: session.end_at });
      continue;
    }
    if (Date.parse(session.start_at) < Date.parse(current.start_at)) current.start_at = session.start_at;
    if (Date.parse(session.end_at) > Date.parse(current.end_at)) current.end_at = session.end_at;
  }
  return ranges;
}

export default function CourseGroupDetail() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { addToast } = useToast();
  const [group, setGroup] = useState<CourseGroup | null>(null);
  const [sessions, setSessions] = useState<CourseGroupSessions[]>([]);
  const [rooms, setRooms] = useState<{ id: string; name: string }[]>([]);
  const [zone, setZone] = useState(DEFAULT_TIME_ZONE);
  const [loading, setLoading] = useState(true);
  const [editNameOpen, setEditNameOpen] = useState(false);
  const [editName, setEditName] = useState("");
  const [savingName, setSavingName] = useState(false);
  const [unmergeOpen, setUnmergeOpen] = useState(false);
  const [unmerging, setUnmerging] = useState(false);

  const roomByID = useMemo(() => new Map(rooms.map((room) => [room.id, room.name])), [rooms]);
  const mergedTimeBySessionDate = useMemo(() => mergedTimeByDate(sessions, zone), [sessions, zone]);

  function openEditName() {
    if (!group) return;
    setEditName(group.name);
    setEditNameOpen(true);
  }

  async function handleNameSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!id || !group || savingName) return;
    const name = editName.trim();
    if (!name) {
      addToast("error", "A merged course name is required.");
      return;
    }
    setSavingName(true);
    try {
      const updated = await updateCourseGroup(id, { name });
      setGroup((current) => current ? { ...current, name: updated.name } : current);
      setEditNameOpen(false);
      addToast("success", "Merged course name updated");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Unable to update merged course name");
    } finally {
      setSavingName(false);
    }
  }

  async function handleUnmerge() {
    if (!id || !group || unmerging) return;
    setUnmerging(true);
    try {
      await deleteCourseGroup(id);
      addToast("success", `"${group.name}" was unmerged`);
      navigate("/courses");
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Unable to unmerge course");
    } finally {
      setUnmerging(false);
      setUnmergeOpen(false);
    }
  }

  useEffect(() => {
    if (!id) return;
    let active = true;
    (async () => {
      try {
        setLoading(true);
        const [groupData, sessionData, roomData, timeData] = await Promise.all([
          getCourseGroup(id),
          getCourseGroupSessions(id),
          getRooms(),
          getInstituteTimeMeta(),
        ]);
        if (!active) return;
        setGroup(groupData);
        setSessions(sessionData);
        setRooms(roomData);
        setZone(timeData.institute_tz || DEFAULT_TIME_ZONE);
      } catch (err) {
        if (active) addToast("error", err instanceof Error ? err.message : "Unable to load merged course");
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [addToast, id]);

  if (loading) {
    return <LoadingSkeleton type="card" lines={2} />;
  }

  if (!group) {
    return <EmptyState message="Merged course not found." action={<Link className="text-sm text-[var(--color-wi-primary)] underline" to="/courses">Return to courses</Link>} />;
  }

  return (
    <div className="w-full space-y-8">
      <Link to="/courses" className="inline-flex items-center gap-2 text-sm text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text)]">
        <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        Courses
      </Link>

      <div className="flex flex-wrap items-start justify-between gap-4 border-b border-[var(--color-wi-line)] pb-6">
        <div>
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="rounded-sm bg-[var(--color-wi-selected)] px-2 py-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Merged course</span>
            <span className="text-sm text-[var(--color-wi-faint)]">{group.members.length} source courses</span>
          </div>
          <PageHeading className="mb-0">{group.name}</PageHeading>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--color-wi-text-light)]">One schedule view for both source courses. Attendance, absence rules, legacy sync, students, and source-course edits remain attached to their original course.</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" onClick={openEditName}>Edit name</Button>
          <Button variant="danger" size="sm" onClick={() => setUnmergeOpen(true)}>Unmerge course</Button>
          <Button variant="secondary" size="sm" onClick={() => window.location.reload()}>Refresh</Button>
        </div>
      </div>

      <section aria-labelledby="source-courses-heading">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 id="source-courses-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Source courses</h2>
          <span className="text-sm text-[var(--color-wi-text-light)]">Edit details and sessions from the source course</span>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          {group.members.map((member) => (
            <article key={member.id} className="rounded-md border border-[var(--color-wi-line)] bg-white p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="font-mono text-xs text-[var(--color-wi-text-light)]">{member.code}</p>
                  <h3 className="mt-1 text-base font-semibold text-[var(--color-wi-text)]">{member.name || member.subject_name || "Unnamed course"}</h3>
                  <p className="mt-1 text-sm text-[var(--color-wi-text-light)]">{member.subject_code} · {member.subject_name}</p>
                </div>
                {member.legacy_course_id ? <span className="rounded-sm border border-[var(--color-wi-blue-soft)] bg-[var(--color-wi-blue-soft)] px-2 py-1 text-xs text-[var(--color-wi-primary)]">Legacy sync</span> : null}
              </div>
              <div className="mt-4 flex flex-wrap gap-2">
                {member.teachers.map((teacher) => <span key={teacher.id} className="rounded-sm border border-[var(--color-wi-line)] bg-[var(--color-wi-row-alt)] px-2 py-1 text-xs text-[var(--color-wi-text-light)]">{displayTeacherName(teacher.full_name ?? null, teacher.username)}</span>)}
              </div>
              <Link to={`/courses/${member.id}`} className="mt-4 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-wi-primary)] hover:underline">
                Open source course <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
              </Link>
            </article>
          ))}
        </div>
      </section>

      <section aria-labelledby="merged-teachers-heading">
        <div className="mb-3 flex items-center justify-between gap-3">
          <h2 id="merged-teachers-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Teachers</h2>
          <span className="text-sm text-[var(--color-wi-text-light)]">{group.teachers.length} across both courses</span>
        </div>
        <div className="flex flex-wrap gap-2">
          {group.teachers.map((teacher) => (
            <span key={teacher.id} className="rounded-sm border border-[var(--color-wi-blue-soft)] bg-[var(--color-wi-blue-soft)] px-3 py-1.5 text-sm text-[var(--color-wi-primary)]">
              {displayTeacherName(teacher.full_name, teacher.username)} <span className="text-xs opacity-75">· {teacher.course_codes.join(" + ")}</span>
            </span>
          ))}
        </div>
      </section>

      <section aria-labelledby="merged-schedule-heading">
        <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
          <h2 id="merged-schedule-heading" className="text-lg font-semibold text-[var(--color-wi-text)]">Schedule</h2>
          <span className="text-sm text-[var(--color-wi-text-light)]">{sessions.length} sessions · TZ: {zone}</span>
        </div>
        <p className="mb-3 text-sm text-[var(--color-wi-text-light)]">Merged time shows the full daily span across this merged course; source times remain listed per course below.</p>
        <div className="overflow-x-auto rounded-md border border-[var(--color-wi-line)]">
          <table className="w-full min-w-[760px] text-[13px]">
            <caption className="sr-only">Combined schedule for {group.name}</caption>
            <thead className="bg-[var(--color-wi-row-alt)]">
              <tr className="border-b border-[var(--color-wi-line)] text-left text-[var(--color-wi-text-light)]">
                <th scope="col" className="px-4 py-3 font-semibold">Date</th>
                <th scope="col" className="px-4 py-3 font-semibold">Begin</th>
                <th scope="col" className="px-4 py-3 font-semibold">End</th>
                <th scope="col" className="px-4 py-3 font-semibold">Duration</th>
                <th scope="col" className="px-4 py-3 font-semibold">Merged time</th>
                <th scope="col" className="px-4 py-3 font-semibold">Source</th>
                <th scope="col" className="px-4 py-3 font-semibold">Teacher</th>
                <th scope="col" className="px-4 py-3 font-semibold">Classroom</th>
              </tr>
            </thead>
            <tbody>
              {sessions.map((session, index) => {
                const duration = minutesBetween(session.start_at, session.end_at);
                const mergedRange = mergedTimeBySessionDate.get(utcISOToZoneDate(session.start_at, zone) ?? "");
                return (
                  <tr key={session.id} className={`border-b border-[var(--color-wi-line)] last:border-0 ${index % 2 === 1 ? "bg-[var(--color-wi-row-alt)]/40" : "bg-white"}`}>
                    <td className="whitespace-nowrap px-4 py-3 font-medium text-[var(--color-wi-text)]">{displayDate(session.start_at, zone)}</td>
                    <td className="whitespace-nowrap px-4 py-3 font-mono text-[var(--color-wi-text-light)]">{displayTime(session.start_at, zone)}</td>
                    <td className="whitespace-nowrap px-4 py-3 font-mono text-[var(--color-wi-text-light)]">{displayTime(session.end_at, zone)}</td>
                    <td className="whitespace-nowrap px-4 py-3 font-mono text-[var(--color-wi-text-light)]">{duration === null ? "—" : fmtDuration(duration)}</td>
                    <td className="whitespace-nowrap px-4 py-3 font-mono text-[var(--color-wi-primary)]">
                      {mergedRange ? `${displayTime(mergedRange.start_at, zone)}–${displayTime(mergedRange.end_at, zone)}` : "—"}
                    </td>
                    <td className="px-4 py-3"><Link to={`/courses/${session.course_id}`} className="font-mono text-xs text-[var(--color-wi-primary)] hover:underline">{session.course_code}</Link></td>
                    <td className="px-4 py-3 text-[var(--color-wi-text-light)]">{session.teacher_name}</td>
                    <td className="px-4 py-3 text-[var(--color-wi-text-light)]">{session.room_id ? roomByID.get(session.room_id) ?? "Unknown" : "Not set"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {sessions.length === 0 ? <EmptyState message="No sessions have been scheduled on either source course." /> : null}
        </div>
      </section>

      {editNameOpen ? (
        <Modal
          title="Edit merged course"
          onClose={() => setEditNameOpen(false)}
          footer={
            <>
              <Button variant="secondary" onClick={() => setEditNameOpen(false)} disabled={savingName}>Cancel</Button>
              <Button variant="primary" type="submit" form="edit-merged-course-form" loading={savingName}>Save changes</Button>
            </>
          }
        >
          <form id="edit-merged-course-form" className="space-y-4" onSubmit={(event) => void handleNameSubmit(event)}>
            <div>
              <label htmlFor="merged-course-name" className="mb-1 block text-sm font-medium text-[var(--color-wi-text)]">Merged course name</label>
              <Input
                id="merged-course-name"
                value={editName}
                onChange={(event) => setEditName(event.target.value)}
                autoFocus
                required
              />
              <p className="mt-2 text-xs leading-5 text-[var(--color-wi-text-light)]">This changes the merged view name only. Source course names, sessions, teachers, students, and absence rules stay attached to their original courses.</p>
            </div>
          </form>
        </Modal>
      ) : null}

      <ConfirmModal
        open={unmergeOpen}
        title="Unmerge course?"
        message={`Remove "${group.name}" as a merged view? The source courses, sessions, teachers, students, and existing absence records will remain unchanged. New absence requests will use each source course's separate absence quota again.`}
        confirmLabel="Unmerge course"
        variant="danger"
        loading={unmerging}
        onConfirm={() => void handleUnmerge()}
        onCancel={() => setUnmergeOpen(false)}
      />
    </div>
  );
}
