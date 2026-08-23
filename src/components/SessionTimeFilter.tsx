import Button from "./ui/Button";
import Input from "./ui/Input";
import {
  isSessionTimeFilterActive,
  validateSessionTimeFilter,
  type SessionTimeFilter as SessionTimeFilterValue,
} from "@/features/scheduling/domain/sessionTimeRange";

type SessionTimeFilterProps = Readonly<{
  value: SessionTimeFilterValue;
  onChange: (value: SessionTimeFilterValue) => void;
  onClear: () => void;
  idPrefix: string;
}>;

export default function SessionTimeFilter({
  value,
  onChange,
  onClear,
  idPrefix,
}: SessionTimeFilterProps) {
  const error = validateSessionTimeFilter(value);
  const active = isSessionTimeFilterActive(value);
  const errorId = `${idPrefix}-error`;

  return (
    <fieldset
      className="rounded-sm border border-wi-line bg-white px-3 py-2.5"
      aria-label="Filter sessions by time"
    >
      <legend className="mb-1.5 text-xs font-semibold text-[var(--color-wi-text)]">
        Session time
      </legend>
      <div className="flex flex-wrap items-end gap-2.5">
        <div>
          <label
            htmlFor={`${idPrefix}-from`}
            className="mb-1 block text-xs font-medium text-[var(--color-wi-text-light)]"
          >
            From
          </label>
          <Input
            id={`${idPrefix}-from`}
            type="time"
            size="sm"
            value={value.from}
            onChange={(event) =>
              onChange({ ...value, from: event.target.value })
            }
            error={Boolean(error)}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
          />
        </div>
        <div>
          <label
            htmlFor={`${idPrefix}-to`}
            className="mb-1 block text-xs font-medium text-[var(--color-wi-text-light)]"
          >
            To
          </label>
          <Input
            id={`${idPrefix}-to`}
            type="time"
            size="sm"
            value={value.to}
            onChange={(event) => onChange({ ...value, to: event.target.value })}
            error={Boolean(error)}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
          />
        </div>
        {active ? (
          <Button variant="ghost" size="sm" type="button" onClick={onClear}>
            Clear
          </Button>
        ) : null}
        <p
          className="basis-full text-xs text-[var(--color-wi-faint)]"
          id={error ? errorId : undefined}
          role={error ? "alert" : undefined}
        >
          {error ?? "Sessions must fit fully inside this local-time window."}
        </p>
      </div>
    </fieldset>
  );
}
