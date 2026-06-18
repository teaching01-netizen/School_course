import { Calendar, AlertTriangle, Users } from 'lucide-react';

type MetricCardsProps = {
  todaySessionCount: number;
  needAttention: number;
  totalAbsences: number;
};

export default function MetricCards({ todaySessionCount, needAttention, totalAbsences }: MetricCardsProps) {
  return (
    <div className="mb-4 grid grid-cols-3 gap-3">
      <div className="rounded border border-gray-200 bg-white px-3 py-2.5">
        <div className="flex items-center gap-2.5">
          <Calendar className="h-5 w-5 shrink-0 text-gray-400" />
          <div>
            <p className="text-xl font-bold text-gray-900">{todaySessionCount}</p>
            <p className="text-[12px] text-gray-500 leading-tight">Today sessions</p>
          </div>
        </div>
      </div>
      <div className="rounded border border-gray-200 bg-white px-3 py-2.5">
        <div className="flex items-center gap-2.5">
          <AlertTriangle className={`h-5 w-5 shrink-0 ${needAttention > 0 ? 'text-amber-500' : 'text-gray-300'}`} />
          <div>
            <p className={`text-xl font-bold ${needAttention > 0 ? 'text-amber-600' : 'text-gray-400'}`}>
              {needAttention}
            </p>
            <p className="text-[12px] text-gray-500 leading-tight">Need attention</p>
          </div>
        </div>
      </div>
      <div className="rounded border border-gray-200 bg-white px-3 py-2.5">
        <div className="flex items-center gap-2.5">
          <Users className={`h-5 w-5 shrink-0 ${totalAbsences > 0 ? 'text-red-500' : 'text-gray-300'}`} />
          <div>
            <p className={`text-xl font-bold ${totalAbsences > 0 ? 'text-red-600' : 'text-gray-400'}`}>
              {totalAbsences}
            </p>
            <p className="text-[12px] text-gray-500 leading-tight">Total absences</p>
          </div>
        </div>
      </div>
    </div>
  );
}
