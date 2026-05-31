import { useEffect, useMemo, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { addDays, format, startOfWeek } from 'date-fns';
import { ApiRequestError, apiJson } from '../api/client';
import { useToast } from '../hooks/useToast';
import { endOfLocalDay, startOfLocalDay } from '../utils/time';
import ScheduleSessionCard from '../components/ScheduleSessionCard';
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

type Teacher = { id: string; username: string; role: 'Admin' | 'Teacher' };
type Course = { id: string; code: string; name: string; teacher_name?: string; subject_code?: string; subject_name?: string; student_count?: number | null };
type Room = { id: string; name: string; capacity: number | null };
type Session = { id: string; course_id: string; room_id: string; teacher_id: string; start_at: string; end_at: string };

export default function TeacherProfile() {
  const { id } = useParams<{ id: string }>();
  const { addToast } = useToast();
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [courses, setCourses] = useState<Course[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(false);

  const teacher = useMemo(() => teachers.find((t) => t.id === id) ?? null, [id, teachers]);

  const courseById = useMemo(() => new Map(courses.map((c) => [c.id, c])), [courses]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);

  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date(), { weekStartsOn: 1 }));
  const weekEnd = useMemo(() => addDays(weekStart, 6), [weekStart]);

  const load = async () => {
    if (!id) return;
    try {
      setLoading(true);
      const start = startOfLocalDay(weekStart).toISOString();
      const end = endOfLocalDay(weekEnd).toISOString();
      const [teacherItems, courseItems, roomItems, sessionItems] = await Promise.all([
        apiJson<Teacher[]>('/api/v1/users?role=Teacher', { method: 'GET' }),
        apiJson<Course[]>('/api/v1/courses', { method: 'GET' }),
        apiJson<Room[]>('/api/v1/rooms', { method: 'GET' }),
        apiJson<Session[]>(`/api/v1/sessions?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`, { method: 'GET' }),
      ]);
      setTeachers(teacherItems);
      setCourses(courseItems);
      setRooms(roomItems);
      setSessions(sessionItems.filter((s) => s.teacher_id === id));
    } catch (err) {
      if (err instanceof ApiRequestError && err.status === 401) {
        addToast('error', 'Please sign in to view teacher profile');
        return;
      }
      addToast('error', err instanceof Error ? err.message : 'Failed to load teacher profile');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, weekStart]);

  const days = ['MON', 'TUE', 'WED', 'THU', 'FRI'];
  const timeSlots = ['09:00', '10:00', '11:00', '12:00', '13:00', '14:00', '15:00', '16:00'];

  const sessionsByWeekdayAndHour = useMemo(() => {
    const map = new Map<string, Session[]>();
    for (const s of sessions) {
      const d = new Date(s.start_at);
      const weekday = d.getDay(); // 0=Sun..6=Sat
      const hour = format(d, 'HH:00');
      const key = `${weekday}-${hour}`;
      const group = map.get(key);
      if (group) {
        group.push(s);
      } else {
        map.set(key, [s]);
      }
    }
    return map;
  }, [sessions]);

  const courseRollup = useMemo(() => {
    const byCourse = new Map<string, number>();
    for (const s of sessions) byCourse.set(s.course_id, (byCourse.get(s.course_id) ?? 0) + 1);
    return Array.from(byCourse.entries())
      .map(([courseId, count]) => ({ courseId, count, course: courseById.get(courseId) }))
      .sort((a, b) => (a.course?.code ?? a.courseId).localeCompare(b.course?.code ?? b.courseId));
  }, [courseById, sessions]);

  if (!id) {
    return (
      <div className="text-center py-20">
        <h2 className="text-xl font-semibold">Teacher not found</h2>
        <Link to="/teachers" className="text-[var(--color-wi-primary)] text-sm mt-2 inline-block">Back to Teachers</Link>
      </div>
    );
  }

  return (
    <div>
      <Link to="/teachers" className="text-sm text-gray-500 hover:text-gray-700 mb-2 inline-block">&larr; Back</Link>
      <div className="flex items-start justify-between mb-4">
        <div>
          <PageHeading>{teacher?.username ?? 'Teacher'}</PageHeading>
          <p className="text-sm text-gray-500 font-mono text-xs">{id}</p>
        </div>
        <div className="flex items-center gap-3">
          <Link to="/users" className="text-sm text-[var(--color-wi-primary)] hover:underline">Manage account</Link>
          <Button variant="secondary" size="md" onClick={() => void load()}>Reload</Button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="border border-gray-200 p-4">
          <h3 className="text-sm font-semibold mb-2">Profile</h3>
          <div className="space-y-1 text-sm">
            <div className="flex justify-between"><span className="text-gray-500">Username</span> <span className="font-mono text-xs">{teacher?.username ?? '-'}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">Role</span> <span>{teacher?.role ?? 'Teacher'}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">Week</span> <span>{format(weekStart, 'yyyy-MM-dd')} → {format(weekEnd, 'yyyy-MM-dd')}</span></div>
          </div>
        </div>
        <div className="border border-gray-200 p-4 lg:col-span-2">
          <h3 className="text-sm font-semibold mb-2">Courses This Week ({courseRollup.length})</h3>
          <div className="overflow-x-auto"><table className="w-full text-[13px]">
            <thead>
              <tr className="border-b-2 border-gray-300">
                <th className="text-left py-1 px-2 font-semibold">C-ID</th>
                <th className="text-left py-1 px-2 font-semibold">Course</th>
                <th className="text-left py-1 px-2 font-semibold">Sessions</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={3}><LoadingSkeleton type="table" lines={2} /></td></tr>
              ) : courseRollup.length === 0 ? (
                <tr><td colSpan={3} className="py-4 text-center text-sm text-gray-500">No sessions this week.</td></tr>
              ) : courseRollup.map((c) => (
                  <tr key={c.courseId} className="border-b border-gray-200 hover:bg-gray-50">
                    <td className="py-1 px-2"><Link to={`/courses/${c.courseId}`} className="text-[var(--color-wi-primary)] hover:underline font-mono text-xs">{c.course?.code ?? c.courseId}</Link></td>
                    <td className="py-1 px-2">{c.course?.name ?? c.courseId}</td>
                    <td className="py-1 px-2">{c.count}</td>
                  </tr>
                ))}
            </tbody>
          </table></div>
        </div>
      </div>

      <div className="border border-gray-200 p-4 mt-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold">Weekly Schedule</h3>
          <div className="flex items-center gap-1.5">
            <Button variant="ghost" size="sm" onClick={() => setWeekStart((prev) => addDays(prev, -7))}>&lsaquo; Prev</Button>
            <Button variant="ghost" size="sm" onClick={() => setWeekStart(startOfWeek(new Date(), { weekStartsOn: 1 }))}>Today</Button>
            <Button variant="ghost" size="sm" onClick={() => setWeekStart((prev) => addDays(prev, 7))}>Next &rsaquo;</Button>
            <span className="text-xs text-gray-500 ml-1 font-mono">
              {format(weekStart, 'MMM d')} – {format(weekEnd, 'MMM d, yyyy')}
            </span>
          </div>
        </div>
        <div className="overflow-x-auto"><table className="w-full text-[12px] border border-gray-200">
            <thead>
              <tr className="bg-gray-50">
                <th className="text-left py-1 px-1 font-semibold border-r border-gray-200 w-12">Time</th>
                {days.map((d) => <th key={d} className="text-center py-1 px-1 font-semibold border-r border-gray-200 min-w-[100px]">{d}</th>)}
              </tr>
            </thead>
            <tbody>
              {timeSlots.map((slot) => (
                <tr key={slot} className="border-b border-gray-200">
                  <td className="py-1 px-1 text-xs text-gray-500 font-medium border-r border-gray-200">{slot}</td>
                  {[1,2,3,4,5].map((day) => {
                    const sessList = sessionsByWeekdayAndHour.get(`${day}-${slot}`) ?? [];
                    return (
                      <td key={day} className="px-1 py-1 border-r border-gray-200 align-top">
                        {sessList.length > 0 ? (
                          <div className="space-y-0.5">
                            {sessList.map((sess) => {
                              const course = courseById.get(sess.course_id);
                              const room = roomById.get(sess.room_id);
                              return (
                                <ScheduleSessionCard
                                  key={sess.id}
                                  session={sess}
                                  course={course}
                                  room={room}
                                  teacherName={teacher?.username}
                                />
                              );
                            })}
                          </div>
                        ) : loading ? (
                          <div className="animate-pulse space-y-1.5">
                            <div className="h-7 bg-gray-100 rounded-sm" />
                            <div className="h-7 bg-gray-100 rounded-sm w-3/4" />
                          </div>
                        ) : null}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table></div>
      </div>
    </div>
  );
}
