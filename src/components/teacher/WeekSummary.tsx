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
    <div className="flex flex-wrap gap-1.5">
      <span className="inline-flex items-center gap-1 rounded-sm border border-gray-200 px-2 py-1 text-[11px] text-gray-700">
        <Calendar className="h-3 w-3 text-gray-400" />
        {totalSessions} {totalSessions === 1 ? 'class' : 'classes'}
      </span>
      <span className={`inline-flex items-center gap-1 rounded-sm border px-2 py-1 text-[11px] ${
        totalAbsences > 0 ? 'border-red-200 text-red-700' : 'border-gray-200 text-gray-500'
      }`}>
        <AlertTriangle className={`h-3 w-3 ${totalAbsences > 0 ? 'text-red-500' : 'text-gray-300'}`} />
        {totalAbsences} {totalAbsences === 1 ? 'absence' : 'absences'}
      </span>
      <span className={`inline-flex items-center gap-1 rounded-sm border px-2 py-1 text-[11px] ${
        totalSitIns > 0 ? 'border-amber-200 text-amber-700' : 'border-gray-200 text-gray-500'
      }`}>
        <Users className={`h-3 w-3 ${totalSitIns > 0 ? 'text-amber-500' : 'text-gray-300'}`} />
        {totalSitIns} {totalSitIns === 1 ? 'visitor' : 'visitors'}
      </span>
      <span className="inline-flex items-center gap-1 rounded-sm border border-gray-200 px-2 py-1 text-[11px] text-gray-700">
        <CheckCircle className="h-3 w-3 text-green-500" />
        {attendanceRate}% attendance
      </span>
    </div>
  );
}
