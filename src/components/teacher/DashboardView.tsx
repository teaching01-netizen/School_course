import { useMemo, useState } from 'react';
import { format } from 'date-fns';
import { Calendar } from 'lucide-react';
import type { TeacherDashboardResponse, TeacherDashboardSession } from '../../types';
import DayTimeline from './DayTimeline';
import MetricCards from './MetricCards';
import CourseDashboardCard from './CourseDashboardCard';
import TeacherDashboardTable from './TeacherDashboardTable';

type DashboardViewProps = {
  data: TeacherDashboardResponse;
  weekStart: Date;
  onBackToToday?: () => void;
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

export default function DashboardView({ data, weekStart, onBackToToday }: DashboardViewProps) {
  const [showGrid, setShowGrid] = useState(false);

  const today = useMemo(() => new Date(), []);
  const todayDateStr = useMemo(() => yyyyMmDd(today), [today]);

  const isCurrentWeek = useMemo(() => {
    const todayWeekStart = yyyyMmDd(new Date(today.getFullYear(), today.getMonth(), today.getDate() - ((today.getDay() + 6) % 7)));
    return yyyyMmDd(weekStart) === todayWeekStart;
  }, [weekStart, todayDateStr, today]);

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
      {/* Date context header */}
      <div className="mb-3 flex items-center justify-between">
        <p className="text-[13px] text-gray-600">
          <span className="font-semibold text-gray-900">{format(today, 'EEE, d MMM yyyy')}</span>
        </p>
        {!isCurrentWeek && onBackToToday ? (
          <button
            type="button"
            onClick={onBackToToday}
            className="text-[12px] font-medium text-[var(--color-wi-primary)] hover:underline"
          >
            Back to Today
          </button>
        ) : null}
      </div>

      <MetricCards
        todaySessionCount={todaySessions.length}
        needAttention={needAttention}
        totalAbsences={todayAbsences}
      />

      {/* Day timeline */}
      <div className="mb-4">
        <DayTimeline sessions={todaySessions} />
      </div>

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
