import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { format, parseISO } from 'date-fns';
import { ApiRequestError, apiJson } from '../api/client';
import { useToast } from '../hooks/useToast';
import WILogo from '../components/WILogo';
import { localDayRangeRFC3339 } from '../utils/time';
import Button from "../components/ui/Button";

type Session = {
  id: string;
  course_id: string;
  room_id: string;
  teacher_id: string;
  start_at: string;
  end_at: string;
};

type Course = { id: string; code: string; name: string };
type Room = { id: string; name: string; capacity: number | null };
type Teacher = { id: string; username: string; role: 'Admin' | 'Teacher' };

export default function Home() {
  const navigate = useNavigate();
  const [date, setDate] = useState(new Date());
  const [roomFilter, setRoomFilter] = useState('');
  const [separatePrint, setSeparatePrint] = useState(false); // UI-only for now

  const [sessions, setSessions] = useState<Session[]>([]);
  const [courses, setCourses] = useState<Course[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [loading, setLoading] = useState(false);

  const { addToast } = useToast();
  const dateStr = format(date, 'yyyy-MM-dd');
  const courseById = useMemo(() => new Map(courses.map((c) => [c.id, c])), [courses]);
  const roomById = useMemo(() => new Map(rooms.map((r) => [r.id, r])), [rooms]);
  const teacherById = useMemo(() => new Map(teachers.map((t) => [t.id, t])), [teachers]);

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
      if (err instanceof ApiRequestError && err.status === 401) {
        addToast('error', 'Please sign in to load schedule');
        return;
      }
      addToast('error', err instanceof Error ? err.message : 'Failed to load schedule');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dateStr]);

  const roomsView = useMemo(() => {
    const byRoom = new Map<string, Session[]>();
    for (const s of sessions) {
      const arr = byRoom.get(s.room_id) ?? [];
      arr.push(s);
      byRoom.set(s.room_id, arr);
    }
    for (const arr of byRoom.values()) arr.sort((a, b) => a.start_at.localeCompare(b.start_at));

    const out = Array.from(byRoom.entries()).map(([roomId, items]) => {
      const r = roomById.get(roomId);
      return { roomId, roomName: r ? r.name : roomId, items };
    });
    out.sort((a, b) => a.roomName.localeCompare(b.roomName));
    return out.filter((room) => !roomFilter || room.roomName.toLowerCase().includes(roomFilter.toLowerCase()));
  }, [roomById, roomFilter, sessions]);

  const dayName = format(date, 'EEE');
  const dayNum = format(date, 'd');
  const monthName = format(date, 'MMM');
  const yearShort = format(date, 'yy');

  return (
    <div>
      {/* Filter Bar */}
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <input
          type="text"
          value={roomFilter}
          onChange={(e) => setRoomFilter(e.target.value)}
          placeholder="Classroom ID"
          className="px-2 py-1 text-sm border border-gray-300 rounded-sm w-40"
        />
        <input
          type="date"
          value={dateStr}
          onChange={(e) => setDate(parseISO(e.target.value))}
          className="px-2 py-1 text-sm border border-gray-300 rounded-sm"
        />
        <label className="flex items-center gap-1 text-sm text-gray-700 cursor-pointer">
          <input
            type="checkbox"
            checked={separatePrint}
            onChange={(e) => setSeparatePrint(e.target.checked)}
            className="w-4 h-4"
          />
          Separate Print
        </label>
        <Button variant="primary" size="md" onClick={() => navigate('/summary')}>Summary</Button>
      </div>

      {/* Room Sections */}
      <div className="space-y-6">
        {loading ? (
          <div className="text-sm text-gray-500">Loading…</div>
        ) : sessions.length === 0 ? (
          <div className="text-sm text-gray-500">No sessions found for {dateStr}.</div>
        ) : (
          roomsView.map((room) => (
          <div key={room.roomId}>
            <WILogo className="mb-2" />
            <h2 className="text-[22px] font-bold text-gray-800 mb-2">
              {room.roomName} on {dayName} {dayNum} {monthName} {yearShort}
            </h2>
            <div className="overflow-x-auto"><table className="w-full text-[13px]">
              <thead>
                <tr className="border-b-2 border-gray-300">
                  <th className="text-left py-2 px-2 font-semibold">Begin</th>
                  <th className="text-left py-2 px-2 font-semibold">End</th>
                  <th className="text-left py-2 px-2 font-semibold">Session</th>
                  <th className="text-left py-2 px-2 font-semibold">Course</th>
                  <th className="text-left py-2 px-2 font-semibold">Teacher</th>
                </tr>
              </thead>
              <tbody>
                {room.items.map((s) => {
                  const course = courseById.get(s.course_id);
                  const teacher = teacherById.get(s.teacher_id);
                  const startLocal = new Date(s.start_at);
                  const endLocal = new Date(s.end_at);
                  return (
                    <tr key={s.id} className="border-b border-gray-200 hover:bg-gray-50">
                      <td className="py-2 px-2">{format(startLocal, 'HH:mm')}</td>
                      <td className="py-2 px-2">{format(endLocal, 'HH:mm')}</td>
                      <td className="py-2 px-2 font-mono text-xs text-gray-600">{s.id.slice(0, 8)}</td>
                      <td className="py-2 px-2">
                        {course ? (
                          <span>
                            <span className="font-mono text-xs text-gray-600">{course.code}</span> — {course.name}
                          </span>
                        ) : (
                          <span className="text-gray-500">{s.course_id}</span>
                        )}
                      </td>
                      <td className="py-2 px-2 font-mono text-xs text-gray-600">{teacher?.username ?? s.teacher_id}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table></div>
          </div>
        ))
        )}

      </div>
    </div>
  );
}
