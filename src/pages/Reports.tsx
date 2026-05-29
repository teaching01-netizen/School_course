import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { format, parseISO } from 'date-fns';
import { ApiRequestError, apiJson } from '../api/client';
import { useToast } from '../hooks/useToast';
import { endOfLocalDay, minutesBetweenRFC3339, startOfLocalDay } from '../utils/time';
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";

type Session = { id: string; course_id: string; room_id: string; teacher_id: string; start_at: string; end_at: string };
type Course = { id: string; code: string; name: string };
type Room = { id: string; name: string; capacity: number | null };
type Teacher = { id: string; username: string; role: 'Admin' | 'Teacher' };

export default function Reports() {
  const [activeReport, setActiveReport] = useState<'daily' | 'teachers' | 'classrooms' | 'courses' | 'absences'>('daily');
  const { addToast } = useToast();

  const today = useMemo(() => new Date(), []);
  const [startDate, setStartDate] = useState(format(new Date(today.getTime() - 6 * 24 * 60 * 60 * 1000), 'yyyy-MM-dd'));
  const [endDate, setEndDate] = useState(format(today, 'yyyy-MM-dd'));

  const [sessions, setSessions] = useState<Session[]>([]);
  const [courses, setCourses] = useState<Course[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [loading, setLoading] = useState(false);

  const load = async () => {
    try {
      setLoading(true);
      const start = startOfLocalDay(parseISO(startDate)).toISOString();
      const end = endOfLocalDay(parseISO(endDate)).toISOString();
      const [sess, courseItems, roomItems, teacherItems] = await Promise.all([
        apiJson<Session[]>(`/api/v1/sessions?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`, { method: 'GET' }),
        apiJson<Course[]>('/api/v1/courses', { method: 'GET' }),
        apiJson<Room[]>('/api/v1/rooms', { method: 'GET' }),
        apiJson<Teacher[]>('/api/v1/users?role=Teacher', { method: 'GET' }),
      ]);
      setSessions(sess);
      setCourses(courseItems);
      setRooms(roomItems);
      setTeachers(teacherItems);
    } catch (err) {
      if (err instanceof ApiRequestError && err.status === 401) {
        addToast('error', 'Please sign in to view reports');
        return;
      }
      addToast('error', err instanceof Error ? err.message : 'Failed to load reports');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [startDate, endDate]);

  const courseById = useMemo(() => new Map(courses.map((c) => [c.id, c])), [courses]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);
  const teacherById = useMemo(() => new Map(teachers.map((t) => [t.id, t])), [teachers]);

  const totalMinutes = useMemo(() => sessions.reduce((sum, s) => sum + minutesBetweenRFC3339(s.start_at, s.end_at), 0), [sessions]);
  const totalHours = totalMinutes / 60;

  const roomUtil = useMemo(() => {
    const byRoom = new Map<string, { sessions: number; minutes: number }>();
    for (const s of sessions) {
      const row = byRoom.get(s.room_id) ?? { sessions: 0, minutes: 0 };
      row.sessions += 1;
      row.minutes += minutesBetweenRFC3339(s.start_at, s.end_at);
      byRoom.set(s.room_id, row);
    }
    return Array.from(byRoom.entries())
      .map(([roomId, v]) => {
        const r = roomById.get(roomId);
        return { id: roomId, name: r?.name ?? roomId, capacity: r?.capacity ?? null, sessions: v.sessions, hours: v.minutes / 60 };
      })
      .sort((a, b) => a.name.localeCompare(b.name));
  }, [roomById, sessions]);

  const teacherLoad = useMemo(() => {
    const byTeacher = new Map<string, { sessions: number; minutes: number }>();
    for (const s of sessions) {
      const row = byTeacher.get(s.teacher_id) ?? { sessions: 0, minutes: 0 };
      row.sessions += 1;
      row.minutes += minutesBetweenRFC3339(s.start_at, s.end_at);
      byTeacher.set(s.teacher_id, row);
    }
    return Array.from(byTeacher.entries())
      .map(([teacherId, v]) => {
        const t = teacherById.get(teacherId);
        return { id: teacherId, username: t?.username ?? teacherId, sessions: v.sessions, hours: v.minutes / 60 };
      })
      .sort((a, b) => a.username.localeCompare(b.username));
  }, [sessions, teacherById]);

  const courseSummary = useMemo(() => {
    const byCourse = new Map<string, { sessions: number; minutes: number }>();
    for (const s of sessions) {
      const row = byCourse.get(s.course_id) ?? { sessions: 0, minutes: 0 };
      row.sessions += 1;
      row.minutes += minutesBetweenRFC3339(s.start_at, s.end_at);
      byCourse.set(s.course_id, row);
    }
    return Array.from(byCourse.entries())
      .map(([courseId, v]) => {
        const c = courseById.get(courseId);
        return { id: courseId, code: c?.code ?? courseId, name: c?.name ?? courseId, sessions: v.sessions, hours: v.minutes / 60 };
      })
      .sort((a, b) => a.code.localeCompare(b.code));
  }, [courseById, sessions]);

  return (
    <div>
      <PageHeading>Report</PageHeading>

      <div className="flex flex-wrap items-center gap-2 mb-4">
        <input type="date" value={startDate} onChange={(e) => setStartDate(e.target.value)} className="px-2 py-1 text-sm border border-gray-300 rounded-sm" />
        <span className="text-sm text-gray-500">to</span>
        <input type="date" value={endDate} onChange={(e) => setEndDate(e.target.value)} className="px-2 py-1 text-sm border border-gray-300 rounded-sm" />
        <Button variant="primary" size="md" onClick={() => void load()}>Reload</Button>
        {loading ? <LoadingSkeleton type="table" lines={3} /> : null}
      </div>

      <div className="flex items-center gap-1 mb-4 border-b border-gray-200">
        {[
          { key: 'daily' as const, label: 'Daily Schedule' },
          { key: 'teachers' as const, label: 'Teacher Load' },
          { key: 'classrooms' as const, label: 'Classroom Utilization' },
          { key: 'courses' as const, label: 'Course Completion' },
          { key: 'absences' as const, label: 'Absences' },
        ].map((r) => (
          <button
            key={r.key}
            onClick={() => setActiveReport(r.key)}
            className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
              activeReport === r.key
                ? 'border-[var(--color-wi-primary)] text-[var(--color-wi-primary)]'
                : 'border-transparent text-gray-500 hover:text-gray-700'
            }`}
          >
            {r.label}
          </button>
        ))}
      </div>

      {activeReport === 'daily' && (
        <div>
          <div className="grid grid-cols-3 gap-4 mb-4">
            <div className="bg-gray-50 border border-gray-200 p-3 rounded-sm">
              <p className="text-xs text-gray-500">Total Sessions</p>
              <p className="text-xl font-bold">{sessions.length}</p>
            </div>
            <div className="bg-gray-50 border border-gray-200 p-3 rounded-sm">
              <p className="text-xs text-gray-500">Total Hours</p>
              <p className="text-xl font-bold text-[var(--color-wi-primary)]">{totalHours.toFixed(1)}</p>
            </div>
            <div className="bg-gray-50 border border-gray-200 p-3 rounded-sm">
              <p className="text-xs text-gray-500">Rooms Used</p>
              <p className="text-xl font-bold">{roomUtil.length}</p>
            </div>
          </div>
          <div className="overflow-x-auto"><table className="w-full text-[13px]">
            <thead>
              <tr className="border-b-2 border-gray-300">
                <th className="text-left py-2 px-2 font-semibold">Room</th>
                <th className="text-left py-2 px-2 font-semibold">Sessions</th>
                <th className="text-left py-2 px-2 font-semibold">Total Hours</th>
                <th className="text-left py-2 px-2 font-semibold">Utilization</th>
              </tr>
            </thead>
            <tbody>
              {roomUtil.map((r) => (
                <tr key={r.name} className="border-b border-gray-200 hover:bg-gray-50">
                  <td className="py-2 px-2">{r.name}</td>
                  <td className="py-2 px-2">{r.sessions}</td>
                  <td className="py-2 px-2">{r.hours.toFixed(1)}h</td>
                  <td className="py-2 px-2">
                    <div className="flex items-center gap-2">
                      <div className="w-24 h-2 bg-gray-200 rounded-sm overflow-hidden">
                        <div className="h-full bg-[var(--color-wi-primary)]" style={{ width: `${Math.min(100, (r.hours / 40) * 100)}%` }} />
                      </div>
                      <span className="text-xs text-gray-500">{Math.round((r.hours / 40) * 100)}%</span>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table></div>
        </div>
      )}

      {activeReport === 'teachers' && (
        <div className="overflow-x-auto"><table className="w-full text-[13px]">
          <thead>
            <tr className="border-b-2 border-gray-300">
              <th className="text-left py-2 px-2 font-semibold">Teacher</th>
              <th className="text-left py-2 px-2 font-semibold">Sessions</th>
              <th className="text-left py-2 px-2 font-semibold">Total Hours</th>
            </tr>
          </thead>
          <tbody>
            {teacherLoad.map((t) => (
              <tr key={t.id} className="border-b border-gray-200 hover:bg-gray-50">
                <td className="py-2 px-2 font-mono text-xs text-gray-600">{t.username}</td>
                <td className="py-2 px-2">{t.sessions}</td>
                <td className="py-2 px-2">{t.hours.toFixed(1)}h</td>
              </tr>
            ))}
          </tbody>
        </table></div>
      )}

      {activeReport === 'classrooms' && (
        <div className="overflow-x-auto"><table className="w-full text-[13px]">
          <thead>
            <tr className="border-b-2 border-gray-300">
              <th className="text-left py-2 px-2 font-semibold">Room</th>
              <th className="text-left py-2 px-2 font-semibold">Capacity</th>
              <th className="text-left py-2 px-2 font-semibold">Sessions</th>
              <th className="text-left py-2 px-2 font-semibold">Hours Used</th>
            </tr>
          </thead>
          <tbody>
            {roomUtil.map((r) => (
              <tr key={r.id} className="border-b border-gray-200 hover:bg-gray-50">
                <td className="py-2 px-2">{r.name}</td>
                <td className="py-2 px-2">{r.capacity}</td>
                <td className="py-2 px-2">{r.sessions}</td>
                <td className="py-2 px-2">{r.hours.toFixed(1)}h</td>
              </tr>
            ))}
          </tbody>
        </table></div>
      )}

      {activeReport === 'courses' && (
        <div className="overflow-x-auto"><table className="w-full text-[13px]">
          <thead>
            <tr className="border-b-2 border-gray-300">
              <th className="text-left py-2 px-2 font-semibold">Code</th>
              <th className="text-left py-2 px-2 font-semibold">Course</th>
              <th className="text-left py-2 px-2 font-semibold">Sessions</th>
              <th className="text-left py-2 px-2 font-semibold">Hours</th>
            </tr>
          </thead>
          <tbody>
            {courseSummary.map((c) => (
              <tr key={c.id} className="border-b border-gray-200 hover:bg-gray-50">
                <td className="py-2 px-2 font-mono text-xs text-gray-600">{c.code}</td>
                <td className="py-2 px-2">{c.name}</td>
                <td className="py-2 px-2">{c.sessions}</td>
                <td className="py-2 px-2">{c.hours.toFixed(1)}h</td>
              </tr>
            ))}
          </tbody>
        </table></div>
      )}

      {activeReport === 'absences' && (
        <div>
          <div className="bg-gray-50 border border-gray-200 p-4 rounded-sm">
            <p className="text-sm text-gray-600">View absence analytics and trends in the dedicated Absence Dashboard.</p>
            <Link to="/absences/dashboard" className="mt-2 inline-flex items-center text-sm font-medium text-[var(--color-wi-primary)] hover:underline">
              Go to Absence Dashboard →
            </Link>
          </div>
        </div>
      )}
    </div>
  );
}
