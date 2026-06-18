import { useMemo } from 'react';
import { format, addDays } from 'date-fns';
import type { TeacherDashboardSession } from '../../types';
import DashboardSessionCell from './DashboardSessionCell';

type TeacherDashboardTableProps = {
  sessions: TeacherDashboardSession[];
  weekStart: Date;
};

const DAY_LABELS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
const TIME_LABELS = [
  '07:00', '08:00',
  '09:00', '10:00', '11:00', '12:00',
  '13:00', '14:00', '15:00', '16:00', '17:00', '18:00',
  '19:00', '20:00', '21:00', '22:00', '23:00', '23:30',
];
const FIRST_HOUR = 7;
const ROW_COUNT = TIME_LABELS.length;

function yyyyMmDd(d: Date): string {
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function getGridPosition(startAt: string, endAt: string) {
  const start = new Date(startAt);
  const end = new Date(endAt);
  const startH = start.getHours() + start.getMinutes() / 60;
  const endH = end.getHours() + end.getMinutes() / 60;

  const rawRowStart = Math.floor(startH - FIRST_HOUR) + 2;
  const rowStart = Math.max(2, Math.min(rawRowStart, ROW_COUNT + 1));

  const rawRowEnd = Math.ceil(endH - FIRST_HOUR) + 2;
  const rowEnd = Math.max(rowStart + 1, Math.min(rawRowEnd, ROW_COUNT + 2));

  return { rowStart, rowSpan: rowEnd - rowStart };
}

export default function TeacherDashboardTable({ sessions, weekStart }: TeacherDashboardTableProps) {
  const today = useMemo(() => yyyyMmDd(new Date()), []);

  const gridCells = useMemo(() => {
    const cells: React.ReactNode[] = [];
    for (let rowIdx = 0; rowIdx < ROW_COUNT; rowIdx++) {
      for (let dayIdx = 0; dayIdx < 7; dayIdx++) {
        const date = addDays(weekStart, dayIdx);
        const dateStr = yyyyMmDd(date);
        const isToday = dateStr === today;
        const isLastCol = dayIdx === 6;
        const isLastRow = rowIdx === ROW_COUNT - 1;

        cells.push(
          <div
            key={`cell-${rowIdx}-${dayIdx}`}
            className={`border-r border-b border-gray-200 ${
              isLastCol ? 'border-r-0' : ''
            } ${isLastRow ? 'border-b-0' : ''} ${
              isToday ? 'bg-blue-50/20' : 'bg-white'
            }`}
            style={{ gridRow: rowIdx + 2, gridColumn: dayIdx + 2 }}
          />,
        );
      }
    }
    return cells;
  }, [weekStart, today]);

  return (
    <div className="overflow-x-auto pb-2">
      <div
        className="grid rounded-sm border border-gray-200 bg-white"
        style={{
          gridTemplateColumns: '60px repeat(7, 1fr)',
          gridTemplateRows: `auto repeat(${ROW_COUNT}, 48px)`,
          minWidth: '800px',
        }}
      >
        {/* Header — Time */}
        <div className="border-r border-b border-gray-200 bg-gray-50 px-2 py-2 text-[11px] font-semibold text-gray-500">
          Time
        </div>

        {/* Header — Day columns */}
        {DAY_LABELS.map((label, i) => {
          const date = addDays(weekStart, i);
          const dateStr = yyyyMmDd(date);
          const isToday = dateStr === today;
          return (
            <div
              key={label}
              className={`border-r border-b border-gray-200 bg-gray-50 px-2 py-2 text-center ${
                i === 6 ? 'border-r-0' : ''
              }`}
            >
              <div
                className={`text-xs font-semibold ${
                  isToday ? 'text-[var(--color-wi-primary)]' : 'text-gray-800'
                }`}
              >
                {label}
              </div>
              <div className="text-[11px] text-gray-500">{format(date, 'd MMM')}</div>
              {isToday ? (
                <div className="mx-auto mt-1 inline-flex items-center rounded-full bg-[var(--color-wi-primary)] px-2 py-0.5 text-[10px] font-semibold text-white">
                  Today
                </div>
              ) : null}
            </div>
          );
        })}

        {/* Time gutter labels */}
        {TIME_LABELS.map((label, rowIdx) => (
          <div
            key={`time-${label}`}
            className="border-r border-b border-gray-200 bg-white px-2 py-1 text-[11px] font-medium text-gray-400"
            style={{ gridRow: rowIdx + 2, gridColumn: 1 }}
          >
            {label}
          </div>
        ))}

        {/* Background grid cells */}
        {gridCells}

        {/* Session cards overlayed on grid */}
        {sessions.map((session) => {
          const d = new Date(session.start_at);
          const dayIndex = d.getDay();
          const { rowStart, rowSpan } = getGridPosition(session.start_at, session.end_at);
          return (
            <div
              key={session.id}
              className="px-0.5 py-0.5"
              style={{
                gridColumn: (dayIndex || 7) + 1,
                gridRow: `${rowStart} / span ${rowSpan}`,
                zIndex: 10,
              }}
            >
              <DashboardSessionCell session={session} />
            </div>
          );
        })}
      </div>
    </div>
  );
}
