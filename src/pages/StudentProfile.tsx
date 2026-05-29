import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { addDays, format, startOfWeek } from 'date-fns';
import Modal from '../components/Modal';
import { apiJson } from '../api/client';
import { useToast } from '../hooks/useToast';
import { endOfLocalDay, startOfLocalDay } from '../utils/time';
import ScheduleSessionCard from '../components/ScheduleSessionCard';
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

type Student = { id: string; wcode: string; full_name: string; notes: string };
type EnrolledCourse = { id: string; code: string; name: string; teacher_name: string; subject_code: string; subject_name: string; student_count: number | null; course_type: string | null };
type Course = { id: string; code: string; name: string; teacher_name?: string; subject_code?: string; subject_name?: string; student_count?: number | null };
type Room = { id: string; name: string; capacity: number | null };
type Session = { id: string; course_id: string; room_id: string; teacher_id: string; start_at: string; end_at: string };

export default function StudentProfile() {
  const { wcode } = useParams<{ wcode: string }>();
  const { addToast } = useToast();

  const [student, setStudent] = useState<Student | null>(null);
  const [loading, setLoading] = useState(true);
  const [enrolledCourses, setEnrolledCourses] = useState<EnrolledCourse[]>([]);
  const [courses, setCourses] = useState<Course[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);

  const [editModal, setEditModal] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({ full_name: '', notes: '' });

  const courseById = useMemo(() => new Map(courses.map((c) => [c.id, c])), [courses]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);
  const enrolledCourseIds = useMemo(() => new Set(enrolledCourses.map((c) => c.id)), [enrolledCourses]);

  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date(), { weekStartsOn: 1 }));
  const weekEnd = useMemo(() => addDays(weekStart, 6), [weekStart]);

  useEffect(() => {
    (async () => {
      if (!wcode) return;
      try {
        setLoading(true);
        const s = await apiJson<Student>(`/api/v1/students/by-wcode?wcode=${encodeURIComponent(wcode)}`, { method: 'GET' });
        setStudent(s);
        setForm({ full_name: s.full_name, notes: s.notes ?? '' });

        const [enrolled, allCourses, roomItems] = await Promise.all([
          apiJson<EnrolledCourse[]>(`/api/v1/students/${encodeURIComponent(s.id)}/courses`, { method: 'GET' }),
          apiJson<Course[]>('/api/v1/courses', { method: 'GET' }),
          apiJson<Room[]>('/api/v1/rooms', { method: 'GET' }),
        ]);
        setEnrolledCourses(enrolled);
        setCourses(allCourses);
        setRooms(roomItems);
      } catch (err) {
        addToast('error', err instanceof Error ? err.message : 'Failed to load student');
      } finally {
        setLoading(false);
      }
    })();
  }, [addToast, wcode]);

  useEffect(() => {
    if (!student) return;
    (async () => {
      try {
        setSessionsLoading(true);
        const start = startOfLocalDay(weekStart).toISOString();
        const end = endOfLocalDay(weekEnd).toISOString();
        const sessionItems = await apiJson<Session[]>(
          `/api/v1/sessions?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`,
          { method: 'GET' },
        );
        // Filter to only sessions in courses the student is enrolled in
        setSessions(sessionItems.filter((s) => enrolledCourseIds.has(s.course_id)));
      } catch (err) {
        addToast('error', err instanceof Error ? err.message : 'Failed to load sessions');
      } finally {
        setSessionsLoading(false);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [student?.id, weekStart, enrolledCourseIds]);

  const handleSave = async () => {
    if (!student) return;
    if (!form.full_name.trim()) {
      addToast('error', 'Name is required');
      return;
    }
    try {
      setSaving(true);
      const updated = await apiJson<Student>(`/api/v1/students/${student.id}`, {
        method: 'PUT',
        body: JSON.stringify({ wcode: student.wcode, full_name: form.full_name, notes: form.notes }),
      });
      setStudent(updated);
      setEditModal(false);
      addToast('success', 'Student updated');
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Update failed');
    } finally {
      setSaving(false);
    }
  };

  const days = ['MON', 'TUE', 'WED', 'THU', 'FRI'];
  const timeSlots = ['09:00', '10:00', '11:00', '12:00', '13:00', '14:00', '15:00', '16:00'];

  const sessionsByWeekdayAndHour = useMemo(() => {
    const map = new Map<string, Session[]>();
    for (const s of sessions) {
      const d = new Date(s.start_at);
      const weekday = d.getDay();
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

  if (loading) return <LoadingSkeleton type="card" lines={3} />;

  if (!student) {
    return (
      <div className="text-center py-20">
        <h2 className="text-xl font-semibold">Student not found</h2>
        <Link to="/students" className="text-[var(--color-wi-primary)] text-sm mt-2 inline-block">
          Back to Students
        </Link>
      </div>
    );
  }

  return (
    <div>
      <Link to="/students" className="text-sm text-gray-500 hover:text-gray-700 mb-2 inline-block">
        &larr; Back to Students
      </Link>

      <div className="flex items-start justify-between mb-4">
        <div>
          <PageHeading>{student.full_name}</PageHeading>
          <p className="text-sm text-gray-500">W-Code: {student.wcode}</p>
        </div>
        <Button variant="secondary" size="md" onClick={() => setEditModal(true)}>Edit</Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Profile Card */}
        <div className="border border-gray-200 rounded-sm bg-white p-4">
          <h3 className="text-sm font-semibold mb-2">Profile</h3>
          <div className="space-y-1 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">Name</span>
              <span>{student.full_name}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">W-Code</span>
              <span className="font-mono text-xs">{student.wcode}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">Notes</span>
              <span className="text-gray-700 truncate max-w-[120px]">{student.notes || '—'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">Courses</span>
              <span>{enrolledCourses.length}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">Week</span>
              <span className="font-mono text-xs">{format(weekStart, 'MMM d')} – {format(weekEnd, 'MMM d, yyyy')}</span>
            </div>
          </div>
        </div>

        {/* Enrolled Courses */}
        <div className="border border-gray-200 rounded-sm bg-white p-4 lg:col-span-2">
          <h3 className="text-sm font-semibold mb-2">Enrolled Courses ({enrolledCourses.length})</h3>
          {enrolledCourses.length === 0 ? (
            <p className="text-sm text-gray-500 py-4 text-center">No courses enrolled.</p>
          ) : (
            <div className="overflow-x-auto"><table className="w-full text-[13px]">
              <thead>
                <tr className="border-b-2 border-gray-300">
                  <th className="text-left py-1 px-2 font-semibold">Code</th>
                  <th className="text-left py-1 px-2 font-semibold">Course</th>
                  <th className="text-left py-1 px-2 font-semibold">Teacher</th>
                  <th className="text-left py-1 px-2 font-semibold">Subject</th>
                  <th className="text-right py-1 px-2 font-semibold">Students</th>
                </tr>
              </thead>
              <tbody>
                {enrolledCourses.map((c) => (
                  <tr key={c.id} className="border-b border-gray-200 hover:bg-gray-50">
                    <td className="py-1 px-2">
                      <Link to={`/courses/${c.id}`} className="text-[var(--color-wi-primary)] hover:underline font-mono text-xs">
                        {c.code}
                      </Link>
                    </td>
                    <td className="py-1 px-2">{c.name}</td>
                    <td className="py-1 px-2">{c.teacher_name || '—'}</td>
                    <td className="py-1 px-2">{c.subject_name || c.subject_code || '—'}</td>
                    <td className="py-1 px-2 text-right">{c.student_count ?? '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table></div>
          )}
        </div>
      </div>

      {/* Weekly Schedule */}
      <div className="border border-gray-200 rounded-sm bg-white p-4 mt-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold">Weekly Schedule</h3>
          <div className="flex items-center gap-1.5">
            <button
              onClick={() => setWeekStart((prev) => addDays(prev, -7))}
              className="px-2 py-1 text-xs border border-gray-300 rounded-sm hover:bg-gray-50"
            >
              &lsaquo; Prev
            </button>
            <button
              onClick={() => setWeekStart(startOfWeek(new Date(), { weekStartsOn: 1 }))}
              className="px-2 py-1 text-xs border border-gray-300 rounded-sm hover:bg-gray-50 font-medium text-[var(--color-wi-primary)]"
            >
              Today
            </button>
            <button
              onClick={() => setWeekStart((prev) => addDays(prev, 7))}
              className="px-2 py-1 text-xs border border-gray-300 rounded-sm hover:bg-gray-50"
            >
              Next &rsaquo;
            </button>
            <span className="text-xs text-gray-500 ml-1 font-mono">
              {format(weekStart, 'MMM d')} – {format(weekEnd, 'MMM d, yyyy')}
            </span>
          </div>
        </div>
        {enrolledCourses.length === 0 ? (
          <p className="text-sm text-gray-500 py-4 text-center">No enrolled courses to display a schedule.</p>
        ) : (
          <div>
            <div className="overflow-x-auto"><table className="w-full text-[12px] border border-gray-200">
              <thead>
                <tr className="bg-gray-50">
                  <th className="text-left py-1 px-1 font-semibold border-r border-gray-200 w-12">Time</th>
                {days.map((d) => (
                  <th key={d} className="text-center py-1 px-1 font-semibold border-r border-gray-200 min-w-[100px]">{d}</th>
                ))}
                </tr>
              </thead>
              <tbody>
                {timeSlots.map((slot) => (
                  <tr key={slot} className="border-b border-gray-200">
                    <td className="py-1 px-1 text-xs text-gray-500 font-medium border-r border-gray-200">{slot}</td>
                    {[1, 2, 3, 4, 5].map((day) => {
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
                                  />
                                );
                              })}
                            </div>
                          ) : sessionsLoading ? (
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
        )}
      </div>

      {editModal && (
        <Modal
          title="Edit Student"
          onClose={() => setEditModal(false)}
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setEditModal(false)}>Cancel</Button>
              <Button variant="primary" size="sm" onClick={handleSave} loading={saving}>
                {saving ? 'Saving…' : 'Save'}
              </Button>
            </>
          }
        >
          <div className="space-y-3">
            <div>
              <label className="block text-xs text-gray-500 mb-1">Name</label>
              <Input size="sm" value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} />
            </div>
            <div>
              <label className="block text-xs text-gray-500 mb-1">Notes</label>
              <textarea
                value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })}
                className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
                rows={5}
              />
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
