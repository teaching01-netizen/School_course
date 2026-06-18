type SummaryBarProps = {
  totalSessions: number;
  totalAbsences: number;
  totalSitIns: number;
};

export default function SummaryBar({ totalSessions, totalAbsences, totalSitIns }: SummaryBarProps) {
  return (
    <div className="mb-4 rounded-sm border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-600">
      <strong className="text-gray-900">{totalSessions}</strong> sessions
      &middot; <strong className="text-gray-900">{totalAbsences}</strong> absences
      &middot; <strong className="text-gray-900">{totalSitIns}</strong> sit-in visitors
    </div>
  );
}
