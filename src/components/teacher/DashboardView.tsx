import { useMemo, useState } from 'react';
import { format } from 'date-fns';
import { Link } from 'react-router-dom';
import { ExternalLink, Calendar } from 'lucide-react';
import type { TeacherDashboardResponse, TeacherDashboardSession } from '../../types';
import MetricCards from './MetricCards';
import CourseDashboardCard from './CourseDashboardCard';
import TeacherDashboardTable from './TeacherDashboardTable';

type DashboardViewProps = {
  data: TeacherDashboardResponse;
  weekStart: Date;
};

type CourseGroup = {
  subjectName: string | null;
  courseCode: string;
  courseName: string;
  courseId: string;
  sessions: TeacherDashboardSession[];
  scheduleSummary: string;
};

const DAY_LABELS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

function yyyyMmDd(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function computeScheduleSummary(sessions: TeacherDashboardSession[]): string {
  if (sessions.length === 0) return '';

  const slots = sessions.map((s) => {
    const d = new Date(s.start_at);
    const dayIndex = d.getDay();
    const dayName = DAY_LABELS[dayIndex === 0 ? 6 : dayIndex - 1];
    const startTime = format(d, 'HH:mm');
    const endTime = format(new Date(s.end_at), 'HH:mm');
    return { dayName, dayIndex, startTime, endTime };
  });

  const uniqueSlots = slots.filter(
    (s, i, arr) => arr.findIndex((x) => x.dayName === s.dayName && x.startTime === s.startTime && x.endTime === s.endTime) === i,
  );

  if (uniqueSlots.length === 0) return '';

  if (uniqueSlots.length <= 3) {
    return uniqueSlots
      .sort((a, b) => a.dayIndex - b.dayIndex)
      .map((s) => `${s.dayName} ${s.startTime}–${s.endTime}`)
      .join(' · ');
  }

  const days = uniqueSlots
    .map((s) => s.dayName)
    .sort((a, b) => DAY_LABELS.indexOf(a) - DAY_LABELS.indexOf(b));
  const first = uniqueSlots[0];
  return `${days.join('/')} ${first.startTime}–${first.endTime}`;
}

function TodayRow({ session }: { session: TeacherDashboardSession }) {
  const absCount = (session.absent_students ?? []).length;
  const sitInCount = (session.sit_in_visitors ?? []).length;
  const displayName = session.subject_name ?? session.course_name;

  const isAbsences = absCount > 0;
  const isSitInsOnly = !isAbsences && sitInCount > 0;

  let dotColor: string;
  let statusText: string;
  if (isAbsences) {
    dotColor = 'bg-red-500';
    statusText = `${absCount} absent`;
  } else if (isSitInsOnly) {
    dotColor = 'bg-amber-500';
    statusText = `${sitInCount} sit-in`;
  } else {
    dotColor = 'bg-green-500';
    statusText = 'All clear';
  }

  return (
    <div className="flex items-center gap-2 py-1">
      <span className={`h-2 w-2 shrink-0 rounded-full ${dotColor}`} />
      <span className="text-[13px] text-gray-700 min-w-0">
        {format(new Date(session.start_at), 'HH:mm')}–{format(new Date(session.end_at), 'HH:mm')}
        {' · '}
        <span className="font-medium">{displayName}</span>
        {' · '}
        <span className={
          isAbsences ? 'text-red-600' : isSitInsOnly ? 'text-amber-600' : 'text-green-600'
        }>
          {statusText}
        </span>
      </span>
      {isAbsences && session.absent_students?.[0] ? (
        <Link
          to={`/absences/${session.absent_students[0].absence_id}`}
          className="shrink-0 text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
        >
          View <ExternalLink className="inline h-3 w-3" />
        </Link>
      ) : null}
    </div>
  );
}

export default function DashboardView({ data, weekStart }: DashboardViewProps) {
  const [showGrid, setShowGrid] = useState(false);

  const todayDateStr = useMemo(() => yyyyMmDd(new Date()), []);

  const todaySessions = useMemo(() => {
    return data.sessions.filter((s) => {
      const sessionDate = yyyyMmDd(new Date(s.start_at));
      return sessionDate === todayDateStr;
    });
  }, [data.sessions, todayDateStr]);

  const needAttention = useMemo(() => {
    return todaySessions.filter(
      (s) => (s.absent_students?.length ?? 0) > 0 || (s.sit_in_visitors?.length ?? 0) > 0,
    ).length;
  }, [todaySessions]);

  const todayAbsences = useMemo(() => {
    return todaySessions.reduce((sum, s) => sum + (s.absent_students?.length ?? 0), 0);
  }, [todaySessions]);

  const courseGroups = useMemo(() => {
    const groups = new Map<string, TeacherDashboardSession[]>();
    for (const session of data.sessions) {
      const existing = groups.get(session.course_id);
      if (existing) {
        existing.push(session);
      } else {
        groups.set(session.course_id, [session]);
      }
    }

    const result: CourseGroup[] = [];
    for (const [courseId, sessions] of groups) {
      const first = sessions[0];
      result.push({
        subjectName: first.subject_name,
        courseCode: first.course_code,
        courseName: first.course_name,
        courseId,
        sessions,
        scheduleSummary: computeScheduleSummary(sessions),
      });
    }

    result.sort((a, b) => {
      const aNeeds = a.sessions.some((s) => (s.absent_students?.length ?? 0) > 0) ? 1 : 0;
      const bNeeds = b.sessions.some((s) => (s.absent_students?.length ?? 0) > 0) ? 1 : 0;
      return bNeeds - aNeeds;
    });

    return result;
  }, [data.sessions]);

  return (
    <>
      <MetricCards
        todaySessionCount={todaySessions.length}
        needAttention={needAttention}
        totalAbsences={todayAbsences}
      />

      {/* Today section */}
      {todaySessions.length > 0 ? (
        <div className="mb-4 rounded border border-gray-200 bg-white px-4 py-2.5">
          <p className="mb-1 text-[11px] font-semibold uppercase tracking-wider text-gray-500">
            Today
          </p>
          <div className="divide-y divide-gray-50">
            {todaySessions.map((s) => (
              <TodayRow key={s.id} session={s} />
            ))}
          </div>
        </div>
      ) : (
        <div className="mb-4 rounded border border-gray-200 bg-white px-4 py-3 text-[13px] text-gray-400">
          No sessions today.
        </div>
      )}

      {/* Course cards */}
      {courseGroups.length === 0 ? (
        <div className="rounded border border-gray-200 bg-white p-8 text-center text-sm text-gray-400">
          No sessions this week.
        </div>
      ) : (
        <div className="space-y-2.5">
          {courseGroups.map((course) => (
            <CourseDashboardCard key={course.courseId} course={course} />
          ))}
        </div>
      )}

      {/* Timetable toggle */}
      <div className="mt-4">
        <button
          type="button"
          onClick={() => setShowGrid((v) => !v)}
          className="flex items-center gap-2 rounded border border-gray-200 bg-white px-4 py-2 text-[13px] font-medium text-gray-700 hover:bg-gray-50"
        >
          <Calendar className="h-4 w-4 text-gray-400" />
          {showGrid ? 'Hide timetable' : 'Show timetable'}
        </button>
        {showGrid ? (
          <div className="mt-2 rounded border border-gray-200 bg-white p-2">
            <TeacherDashboardTable sessions={data.sessions} weekStart={weekStart} />
          </div>
        ) : null}
      </div>
    </>
  );
}
