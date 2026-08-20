import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { format, parseISO } from 'date-fns';
import WILogo from '../components/WILogo';
import { ApiRequestError, apiJson } from '../api/client';
import { localDayRangeRFC3339 } from '../utils/time';
import Button from "../components/ui/Button";

type Session = {
  id: string;
  course_id: string;
  room_id: string | null;
  teacher_id: string;
  start_at: string;
  end_at: string;
};

// Legacy sync can produce sessions without a classroom; they are grouped here.
const UNASSIGNED_ROOM_KEY = 'unassigned';
const roomDisplayName = (roomId: string, name?: string) => name || (roomId === UNASSIGNED_ROOM_KEY ? 'Unassigned' : roomId);

type Course = { id: string; code: string; name: string; subject_name: string };
type Room = { id: string; name: string; capacity: number | null };
type Teacher = { id: string; username: string; full_name: string | null; role: 'Admin' | 'Teacher' };

export default function Summary() {
  const navigate = useNavigate();
  const [date, setDate] = useState(new Date());
  const dateStr = format(date, 'yyyy-MM-dd');
  const [sessions, setSessions] = useState<Session[]>([]);
  const [courses, setCourses] = useState<Course[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [loading, setLoading] = useState(false);

  const load = async () => {
    try {
      setLoading(true);
      const { start, end } = localDayRangeRFC3339(date);
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
      if (err instanceof ApiRequestError && err.status === 401) return;
      // keep UI minimal on errors for print view; empty state will show.
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dateStr]);

  const dayName = format(date, 'EEEE');
  const dayNum = format(date, 'd');
  const monthName = format(date, 'MMMM');
  const yearFull = format(date, 'yyyy');

  const courseById = useMemo(() => new Map(courses.map((c) => [c.id, c])), [courses]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);
  const teacherById = useMemo(() => new Map(teachers.map((t) => [t.id, t])), [teachers]);

  const rows = useMemo(() => {
    const byRoom = new Map<string, Session[]>();
    for (const s of sessions) {
      const key = s.room_id ?? UNASSIGNED_ROOM_KEY;
      const arr = byRoom.get(key) ?? [];
      arr.push(s);
      byRoom.set(key, arr);
    }
    const out: { room: string; time: string; course: string; student: string; teacher: string; isFirst: boolean }[] = [];
    const roomsSorted = Array.from(byRoom.keys()).sort((a, b) => roomDisplayName(a, roomById.get(a)?.name).localeCompare(roomDisplayName(b, roomById.get(b)?.name)));
    for (const roomId of roomsSorted) {
      const rname = roomDisplayName(roomId, roomById.get(roomId)?.name);
      const items = byRoom.get(roomId) ?? [];
      items.sort((a, b) => a.start_at.localeCompare(b.start_at));
      if (items.length === 0) {
        out.push({ room: rname, time: '', course: '', student: '', teacher: '', isFirst: true });
        continue;
      }
      items.forEach((s, idx) => {
        const startLocal = new Date(s.start_at);
        const endLocal = new Date(s.end_at);
        const c = courseById.get(s.course_id);
        const t = teacherById.get(s.teacher_id);
        out.push({
          room: idx === 0 ? rname : '',
          time: `${format(startLocal, 'HH:mm')} - ${format(endLocal, 'HH:mm')}`,
          course: c ? (c.subject_name || c.name) : s.course_id,
          student: '',
          teacher: t ? (t.full_name || t.username) : s.teacher_id,
          isFirst: idx === 0,
        });
      });
    }
    return out;
  }, [courseById, roomById, sessions, teacherById]);

  return (
    <div>
      {/* Top controls */}
      <div className="flex items-center justify-center gap-2 mb-4">
        <input
          type="date"
          value={dateStr}
          onChange={(e) => setDate(parseISO(e.target.value))}
          className="px-2 py-1 text-sm border var(--color-wi-line) rounded-sm"
        />
        <Button variant="secondary" size="md">Search</Button>
        <Button variant="primary" size="md" onClick={() => navigate('/')}>Back</Button>
      </div>

      {/* Centered content */}
      <div className="max-w-3xl mx-auto">
        {/* Logo */}
        <div className="flex justify-center mb-4">
          <WILogo className="scale-150" />
        </div>

        {/* Blue header bar */}
        <div className="bg-[var(--color-wi-primary)] text-white text-center py-2 text-[15px] font-semibold">
          Today&apos; Classes - {dayName} {dayNum} {monthName} {yearFull}
        </div>

        {/* Summary table */}
        <table className="w-full text-[13px] border var(--color-wi-line)">
          <caption className="sr-only">Daily classes summary</caption>
          <thead>
            <tr className="border-b var(--color-wi-line)">
              <th scope="col" className="text-center py-2 px-2 font-semibold border-r var(--color-wi-line) w-[22%]">Room</th>
              <th scope="col" className="text-center py-2 px-2 font-semibold border-r var(--color-wi-line) w-[18%]">Time</th>
              <th scope="col" className="text-center py-2 px-2 font-semibold border-r var(--color-wi-line) w-[26%]">Course</th>
              <th scope="col" className="text-center py-2 px-2 font-semibold border-r var(--color-wi-line) w-[16%]">Student</th>
              <th scope="col" className="text-center py-2 px-2 font-semibold w-[18%]">Teacher</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr><td colSpan={5} className="py-6 text-center text-sm text-[var(--color-wi-text-light)]">Loading…</td></tr>
            ) : rows.length === 0 ? (
              <tr><td colSpan={5} className="py-6 text-center text-sm text-[var(--color-wi-text-light)]">No sessions found.</td></tr>
            ) : rows.map((row, idx) => (
              <tr key={idx} className="border-b var(--color-wi-line)">
                <td className={`py-2 px-2 border-r var(--color-wi-line) text-center font-semibold ${row.isFirst ? '' : ''}`}>
                  {row.room}
                </td>
                <td className="py-2 px-2 border-r var(--color-wi-line) text-center">{row.time}</td>
                <td className="py-2 px-2 border-r var(--color-wi-line) text-center">{row.course}</td>
                <td className="py-2 px-2 border-r var(--color-wi-line) text-center">{row.student}</td>
                <td className="py-2 px-2 text-center">{row.teacher}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
