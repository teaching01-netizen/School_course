import { useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { addDays, format, startOfWeek } from 'date-fns';
import Modal from '../components/Modal';
import { apiJson } from '../api/client';
import { useToast } from '../hooks/useToast';
import { endOfLocalDay, startOfLocalDay } from '../utils/time';
import { groupSessionKey } from '../utils/timezone';
import ScheduleSessionCard from '../components/ScheduleSessionCard';
import PageHeading from "../components/ui/PageHeading";
import Button from "../components/ui/Button";
import Input from "../components/ui/Input";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";

type Student = {
  id: string;
  wcode: string;
  full_name: string;
  notes: string;
  nickname: string;
  school: string;
  level: string;
  year: string;
  student_phone: string;
  email: string;
};
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
  const [instituteTZ, setInstituteTZ] = useState<string | null>(null);
  const zone = instituteTZ ?? 'Asia/Bangkok';

  const [editModal, setEditModal] = useState(false);
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState({
    full_name: '',
    notes: '',
    nickname: '',
    school: '',
    level: '',
    year: '',
    student_phone: '',
    email: '',
  });

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
        setForm({
          full_name: s.full_name,
          notes: s.notes ?? '',
          nickname: s.nickname ?? '',
          school: s.school ?? '',
          level: s.level ?? '',
          year: s.year ?? '',
          student_phone: s.student_phone ?? '',
          email: s.email ?? '',
        });

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

  useEffect(() => {
    (async () => {
      try {
        const meta = await apiJson<{ institute_tz: string }>(`/api/v1/meta/time`, { method: 'GET' });
        setInstituteTZ(meta.institute_tz);
      } catch {
        // Best-effort only.
      }
    })();
  }, []);

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
        body: JSON.stringify({
          wcode: student.wcode,
          full_name: form.full_name,
          notes: form.notes,
          nickname: form.nickname,
          school: form.school,
          level: form.level,
          year: form.year,
          student_phone: form.student_phone,
          email: form.email,
        }),
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
  const timeSlots = Array.from({ length: 24 }, (_, i) => `${String(i).padStart(2, '0')}:00`);

  const sessionsByWeekdayAndHour = useMemo(() => {
    const map = new Map<string, Session[]>();
    for (const s of sessions) {
      const key = groupSessionKey(s.start_at, zone);
      if (!key) continue;
      const group = map.get(key);
      if (group) {
        group.push(s);
      } else {
        map.set(key, [s]);
      }
    }
    return map;
  }, [sessions, zone]);

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
      <Link to="/students" className="text-sm text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)] mb-2 inline-block">
        &larr; Back to Students
      </Link>

      <div className="flex items-start justify-between mb-4">
        <div>
          <PageHeading>{student.full_name}</PageHeading>
          <p className="text-sm text-[var(--color-wi-text-light)]">W-Code: {student.wcode}</p>
        </div>
        <Button variant="secondary" size="md" onClick={() => setEditModal(true)}>Edit</Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Profile Card */}
        <div className="border var(--color-wi-line) rounded-sm bg-white p-4">
          <h3 className="text-sm font-semibold mb-2">Profile</h3>
          <div className="space-y-1 text-sm">
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Name</span>
              <span>{student.full_name}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Nickname</span>
              <span>{student.nickname || '—'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">W-Code</span>
              <span className="font-mono text-xs">{student.wcode}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">School</span>
              <span>{student.school || '—'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Level</span>
              <span>{student.level || '—'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Year</span>
              <span>{student.year || '—'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Phone</span>
              <span>{student.student_phone || '—'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Email</span>
              <span className="truncate max-w-[130px]">{student.email || '—'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Notes</span>
              <span className="text-[var(--color-wi-text-light)] truncate max-w-[120px]">{student.notes || '—'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Courses</span>
              <span>{enrolledCourses.length}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-[var(--color-wi-text-light)]">Week</span>
              <span className="font-mono text-xs">{format(weekStart, 'MMM d')} – {format(weekEnd, 'MMM d, yyyy')}</span>
            </div>
          </div>
        </div>

        {/* Enrolled Courses */}
        <div className="border var(--color-wi-line) rounded-sm bg-white p-4 lg:col-span-2">
          <h3 className="text-sm font-semibold mb-2">Enrolled Courses ({enrolledCourses.length})</h3>
          {enrolledCourses.length === 0 ? (
            <p className="text-sm text-[var(--color-wi-text-light)] py-4 text-center">No courses enrolled.</p>
          ) : (
            <div className="overflow-x-auto"><table className="w-full text-[13px]">
              <caption className="sr-only">Enrolled courses</caption>
              <thead>
                <tr className="border-b var(--color-wi-line)">
                  <th scope="col" className="text-left py-1 px-2 font-semibold">Code</th>
                  <th scope="col" className="text-left py-1 px-2 font-semibold">Course</th>
                  <th scope="col" className="text-left py-1 px-2 font-semibold">Teacher</th>
                  <th scope="col" className="text-left py-1 px-2 font-semibold">Subject</th>
                  <th scope="col" className="text-right py-1 px-2 font-semibold">Students</th>
                </tr>
              </thead>
              <tbody>
                {enrolledCourses.map((c) => (
                  <tr key={c.id} className="border-b var(--color-wi-line) hover:bg-[var(--color-wi-row-alt)]">
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
      <div className="border var(--color-wi-line) rounded-sm bg-white p-4 mt-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold">Weekly Schedule</h3>
          <div className="flex items-center gap-1.5">
            <button
              onClick={() => setWeekStart((prev) => addDays(prev, -7))}
              className="px-2 py-1 text-xs border var(--color-wi-line) rounded-sm hover:bg-[var(--color-wi-row-alt)]"
            >
              &lsaquo; Prev
            </button>
            <button
              onClick={() => setWeekStart(startOfWeek(new Date(), { weekStartsOn: 1 }))}
              className="px-2 py-1 text-xs border var(--color-wi-line) rounded-sm hover:bg-[var(--color-wi-row-alt)] font-medium text-[var(--color-wi-primary)]"
            >
              Today
            </button>
            <button
              onClick={() => setWeekStart((prev) => addDays(prev, 7))}
              className="px-2 py-1 text-xs border var(--color-wi-line) rounded-sm hover:bg-[var(--color-wi-row-alt)]"
            >
              Next &rsaquo;
            </button>
            <span className="text-xs text-[var(--color-wi-text-light)] ml-1 font-mono">
              {format(weekStart, 'MMM d')} – {format(weekEnd, 'MMM d, yyyy')}
            </span>
          </div>
        </div>
        {enrolledCourses.length === 0 ? (
          <p className="text-sm text-[var(--color-wi-text-light)] py-4 text-center">No enrolled courses to display a schedule.</p>
        ) : (
          <div>
            <div className="overflow-x-auto"><table className="w-full text-[12px] border var(--color-wi-line)">
              <caption className="sr-only">Weekly schedule</caption>
              <thead>
                <tr className="bg-[var(--color-wi-row-alt)]">
                  <th scope="col" className="text-left py-1 px-1 font-semibold border-r var(--color-wi-line) w-12">Time</th>
                {days.map((d) => (
                  <th scope="col" key={d} className="text-center py-1 px-1 font-semibold border-r var(--color-wi-line) min-w-[100px]">{d}</th>
                ))}
                </tr>
              </thead>
              <tbody>
                {timeSlots.map((slot) => (
                  <tr key={slot} className="border-b var(--color-wi-line)">
                    <td className="py-1 px-1 text-xs text-[var(--color-wi-text-light)] font-medium border-r var(--color-wi-line)">{slot}</td>
                    {[1, 2, 3, 4, 5].map((day) => {
                      const sessList = sessionsByWeekdayAndHour.get(`${day}-${slot}`) ?? [];
                      return (
                        <td key={day} className="px-1 py-1 border-r var(--color-wi-line) align-top">
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
                                    zone={zone}
                                  />
                                );
                              })}
                            </div>
                          ) : sessionsLoading ? (
                            <div className="animate-pulse space-y-1.5">
                              <div className="h-7 bg-[var(--color-wi-row-alt)] rounded-sm" />
                              <div className="h-7 bg-[var(--color-wi-row-alt)] rounded-sm w-3/4" />
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
              <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Name</label>
              <Input size="sm" value={form.full_name} onChange={(e) => setForm({ ...form, full_name: e.target.value })} />
            </div>
            <div>
              <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Nickname</label>
              <Input size="sm" value={form.nickname} onChange={(e) => setForm({ ...form, nickname: e.target.value })} />
            </div>
            <div>
              <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">School</label>
              <Input size="sm" value={form.school} onChange={(e) => setForm({ ...form, school: e.target.value })} />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Level</label>
                <Input size="sm" value={form.level} onChange={(e) => setForm({ ...form, level: e.target.value })} />
              </div>
              <div>
                <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Year</label>
                <Input size="sm" value={form.year} onChange={(e) => setForm({ ...form, year: e.target.value })} />
              </div>
            </div>
            <div>
              <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Phone</label>
              <Input size="sm" value={form.student_phone} onChange={(e) => setForm({ ...form, student_phone: e.target.value })} />
            </div>
            <div>
              <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Email</label>
              <Input size="sm" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
            </div>
            <div>
              <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Notes</label>
              <textarea
                value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })}
                className="w-full px-2 py-1.5 text-sm border var(--color-wi-line) rounded-sm"
                rows={5}
              />
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
