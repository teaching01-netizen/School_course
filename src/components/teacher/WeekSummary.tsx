import { Calendar, AlertTriangle, Users, CheckCircle } from 'lucide-react';

type WeekSummaryProps = {
  totalSessions: number;
  totalAbsences: number;
  totalSitIns: number;
};

export default function WeekSummary({ totalSessions, totalAbsences, totalSitIns }: WeekSummaryProps) {
  const attendanceRate = totalSessions > 0
    ? Math.round(((totalSessions + totalSitIns - totalAbsences) / (totalSessions + totalSitIns)) * 100)
    : 100;

  return (
    <div className="grid grid-cols-2 gap-1.5 sm:flex sm:flex-wrap">
      <span className="inline-flex items-center justify-center gap-1 rounded-sm border border-[var(--color-wi-line)] px-2 py-1 text-[11px] text-[var(--color-wi-text-light)] sm:justify-start">
        <Calendar className="h-3 w-3 text-[var(--color-wi-text-light)]" />
        {totalSessions} {totalSessions === 1 ? 'class' : 'classes'}
      </span>
      <span className={`inline-flex items-center justify-center gap-1 rounded-sm border px-2 py-1 text-[11px] sm:justify-start ${
        totalAbsences > 0 ? 'border-red-200 text-red-700' : 'border-[var(--color-wi-line)] text-[var(--color-wi-text-light)]'
      }`}>
        <AlertTriangle className={`h-3 w-3 ${totalAbsences > 0 ? 'text-red-500' : 'text-[var(--color-wi-text-light)]'}`} />
        {totalAbsences} {totalAbsences === 1 ? 'absence' : 'absences'}
      </span>
      <span className={`inline-flex items-center justify-center gap-1 rounded-sm border px-2 py-1 text-[11px] sm:justify-start ${
        totalSitIns > 0 ? 'border-amber-200 text-amber-700' : 'border-[var(--color-wi-line)] text-[var(--color-wi-text-light)]'
      }`}>
        <Users className={`h-3 w-3 ${totalSitIns > 0 ? 'text-amber-500' : 'text-[var(--color-wi-text-light)]'}`} />
        {totalSitIns} {totalSitIns === 1 ? 'visitor' : 'visitors'}
      </span>
      <span className="inline-flex items-center justify-center gap-1 rounded-sm border border-[var(--color-wi-line)] px-2 py-1 text-[11px] text-[var(--color-wi-text-light)] sm:justify-start">
        <CheckCircle className="h-3 w-3 text-green-500" />
        {attendanceRate}% attendance
      </span>
    </div>
  );
}
