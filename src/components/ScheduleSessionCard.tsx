import { useRef, useState } from 'react';
import { format } from 'date-fns';

type Session = { id: string; course_id: string; room_id: string | null; start_at: string; end_at: string };
type Course = { id: string; code: string; name: string; teacher_name?: string; subject_code?: string; subject_name?: string; student_count?: number | null };
type Room = { id: string; name: string; capacity: number | null };

interface ScheduleSessionCardProps {
  session: Session;
  course?: Course;
  room?: Room;
  /** Optional teacher name override (e.g., current profile teacher) */
  teacherName?: string;
}

export default function ScheduleSessionCard({ session, course, room, teacherName }: ScheduleSessionCardProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [tooltip, setTooltip] = useState<{ visible: boolean; above: boolean }>({ visible: false, above: true });

  const handleMouseEnter = () => {
    if (ref.current) {
      const rect = ref.current.getBoundingClientRect();
      // ~220px = tooltip height (w-52 ~= 208px + margins/padding)
      setTooltip({ visible: true, above: rect.top > 220 });
    }
  };

  const handleMouseLeave = () => setTooltip({ visible: false, above: true });

  return (
    <div ref={ref} className="relative" onMouseEnter={handleMouseEnter} onMouseLeave={handleMouseLeave}>
      {/* Card */}
      <div className="bg-[color-mix(in_oklab,var(--color-wi-primary)_10%,transparent)] border border-[color-mix(in_oklab,var(--color-wi-primary)_20%,transparent)] p-1 text-[10px] cursor-default">
        <p className="font-semibold">{course?.name ?? session.course_id}</p>
        <p className="text-gray-500">{room?.name ?? session.room_id}</p>
        <p className="text-gray-400">{format(session.start_at, 'HH:mm')}–{format(session.end_at, 'HH:mm')}</p>
      </div>

      {/* Tooltip — flips above/below based on available space */}
      <div
        className={`
          pointer-events-none absolute z-10 w-52 rounded-md border border-gray-200 bg-white px-3 py-2 text-xs shadow-lg
          transition-opacity duration-150
          ${tooltip.visible ? 'opacity-100' : 'opacity-0'}
          ${tooltip.above
            ? 'bottom-full left-1/2 -translate-x-1/2 mb-1.5'
            : 'top-full left-1/2 -translate-x-1/2 mt-1.5'
          }
        `}
      >
        <p className="font-semibold text-gray-800 mb-0.5">{course?.code ?? '—'} – {course?.name ?? '—'}</p>
        <div className="space-y-0.5 text-gray-600">
          <p><span className="text-gray-400">Subject:</span> {course?.subject_name || course?.subject_code || '—'}</p>
          <p><span className="text-gray-400">Teacher:</span> {teacherName ?? course?.teacher_name ?? '—'}</p>
          <p><span className="text-gray-400">Room:</span> {room?.name ?? '—'}{room?.capacity != null ? ` (cap. ${room.capacity})` : ''}</p>
          <p><span className="text-gray-400">Time:</span> {format(session.start_at, 'EEEE HH:mm')}–{format(session.end_at, 'HH:mm')}</p>
        </div>
      </div>
    </div>
  );
}
