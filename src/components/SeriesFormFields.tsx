import Input from "./ui/Input";
import FormField from "./ui/FormField";

const WEEKDAY_LABELS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

interface SeriesFormFieldsProps {
  weekdays: boolean[];
  onWeekdayChange: (idx: number) => void;
  startLocalTime: string;
  onStartLocalTimeChange: (v: string) => void;
  durationMinutes: number;
  onDurationMinutesChange: (v: number) => void;
  useCount: boolean;
  onUseCountChange: (v: boolean) => void;
  count: number;
  onCountChange: (v: number) => void;
  endDate: string;
  onEndDateChange: (v: string) => void;
  startDate?: string;
  onStartDateChange?: (v: string) => void;
  errors?: Record<string, string>;
  touched?: Record<string, boolean>;
  prefix?: string;
}

export default function SeriesFormFields({
  weekdays,
  onWeekdayChange,
  startLocalTime,
  onStartLocalTimeChange,
  durationMinutes,
  onDurationMinutesChange,
  useCount,
  onUseCountChange,
  count,
  onCountChange,
  endDate,
  onEndDateChange,
  startDate,
  onStartDateChange,
  errors = {},
  touched = {},
  prefix = "",
}: SeriesFormFieldsProps) {
  return (
    <>
      <div className="bg-gray-50 rounded-sm p-3 space-y-3">
        <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Schedule</div>
        <FormField name={`${prefix}weekdays`} label="Weekdays (Bangkok)">
          <div className="flex flex-wrap gap-2 text-sm">
            {WEEKDAY_LABELS.map((label, idx) => (
              <label key={label} className="flex items-center gap-1 cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={weekdays[idx]}
                  onChange={() => onWeekdayChange(idx)}
                />
                {label}
              </label>
            ))}
          </div>
        </FormField>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          <FormField
            name={`${prefix}start_local_time`}
            label="Start time (Bangkok)"
            error={errors.start_local_time}
            touched={touched.start_local_time}
            required
          >
            <Input
              type="time"
              size="sm"
              step={300}
              value={startLocalTime}
              onChange={(e) => onStartLocalTimeChange(e.target.value)}
            />
          </FormField>
          <FormField
            name={`${prefix}duration_minutes`}
            label="Duration (minutes)"
            error={errors.duration_minutes}
            touched={touched.duration_minutes}
          >
            <Input
              type="number"
              size="sm"
              value={durationMinutes}
              onChange={(e) => onDurationMinutesChange(Number(e.target.value))}
            />
          </FormField>
          {onStartDateChange ? (
            <FormField
              name={`${prefix}start_date`}
              label="Start date"
              error={errors.start_date}
              touched={touched.start_date}
              required
            >
              <Input
                type="date"
                size="sm"
                value={startDate ?? ""}
                onChange={(e) => onStartDateChange(e.target.value)}
              />
            </FormField>
          ) : null}
        </div>
      </div>

      <div className="bg-gray-50 rounded-sm p-3 space-y-3">
        <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">End</div>
        <div className="flex items-center gap-2 text-sm">
          <label className="flex items-center gap-1 cursor-pointer">
            <input type="checkbox" checked={useCount} onChange={(e) => onUseCountChange(e.target.checked)} />
            End by count (advanced)
          </label>
        </div>

        {useCount ? (
          <FormField name={`${prefix}count`} label="Count (total occurrences)">
            <Input type="number" size="sm" value={count} onChange={(e) => onCountChange(Number(e.target.value))} />
          </FormField>
        ) : (
          <FormField name={`${prefix}end_date`} label="End date">
            <Input type="date" size="sm" value={endDate} onChange={(e) => onEndDateChange(e.target.value)} />
          </FormField>
        )}
      </div>
    </>
  );
}
