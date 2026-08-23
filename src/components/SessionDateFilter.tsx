import Button from "./ui/Button";
import Input from "./ui/Input";
import {
  isSessionDateFilterActive,
  validateSessionDateFilter,
  type SessionDateFilter as SessionDateFilterValue,
} from "@/features/scheduling/domain/sessionDateRange";

type SessionDateFilterProps = Readonly<{
  value: SessionDateFilterValue;
  onChange: (value: SessionDateFilterValue) => void;
  onClear: () => void;
  idPrefix: string;
}>;

export default function SessionDateFilter({
  value,
  onChange,
  onClear,
  idPrefix,
}: SessionDateFilterProps) {
  const error = validateSessionDateFilter(value);
  const active = isSessionDateFilterActive(value);
  const errorId = `${idPrefix}-error`;

  return (
    <fieldset
      className="rounded-sm border border-wi-line bg-white px-3 py-2.5"
      aria-label="Filter sessions by date"
    >
      <legend className="mb-1.5 text-xs font-semibold text-[var(--color-wi-text)]">
        Session dates
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
            type="date"
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
            type="date"
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
          {error ?? "Sessions must fall within this local calendar date range."}
        </p>
      </div>
    </fieldset>
  );
}
