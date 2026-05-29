import Button from "./ui/Button";
import Input from "./ui/Input";

interface ScheduleFiltersProps {
  startDate: string;
  endDate: string;
  startTime: string;
  endTime: string;
  viewMode: "week" | "table";
  onChangeStartDate: (v: string) => void;
  onChangeEndDate: (v: string) => void;
  onChangeStartTime: (v: string) => void;
  onChangeEndTime: (v: string) => void;
  onRefresh: () => void;
  onViewModeChange: (m: "week" | "table") => void;
  onOpenCreate: () => void;
  onOpenSeries: () => void;
}

export default function ScheduleFilters({
  startDate, endDate, startTime, endTime, viewMode,
  onChangeStartDate, onChangeEndDate, onChangeStartTime, onChangeEndTime,
  onRefresh, onViewModeChange, onOpenCreate, onOpenSeries,
}: ScheduleFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-2 mb-4">
      <Input type="date" size="sm" value={startDate} onChange={(e) => onChangeStartDate(e.target.value)} className="w-36" />
      <Input type="date" size="sm" value={endDate} onChange={(e) => onChangeEndDate(e.target.value)} className="w-36" />
      <Input type="time" size="sm" value={startTime} onChange={(e) => onChangeStartTime(e.target.value)} step={300} className="w-28" />
      <Input type="time" size="sm" value={endTime} onChange={(e) => onChangeEndTime(e.target.value)} step={300} className="w-28" />
      <Button variant="secondary" size="sm" onClick={onRefresh}>Refresh</Button>
      <div className="flex border border-gray-300 rounded-sm overflow-hidden">
        <button
          onClick={() => onViewModeChange("week")}
          className={`px-3 py-1 text-sm ${viewMode === "week" ? "bg-[var(--color-wi-primary)] text-white" : "bg-white text-gray-700 hover:bg-gray-50"}`}
        >
          Week
        </button>
        <button
          onClick={() => onViewModeChange("table")}
          className={`px-3 py-1 text-sm ${viewMode === "table" ? "bg-[var(--color-wi-primary)] text-white" : "bg-white text-gray-700 hover:bg-gray-50"}`}
        >
          Table
        </button>
      </div>
      <Button variant="primary" size="sm" onClick={onOpenCreate}>Create Session</Button>
      <Button variant="secondary" size="sm" onClick={onOpenSeries}>Create Series</Button>
    </div>
  );
}
