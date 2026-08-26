import type { CrmRowInfo } from "../../types/crossStudy";

type Props = {
  crmRow: CrmRowInfo;
  selected: boolean;
  onSelect: () => void;
};

export default function CrossStudyCrmRowCard({ crmRow, selected, onSelect }: Props) {
  return (
    <div className={`rounded-sm border overflow-hidden ${selected ? "border-blue-500 ring-1 ring-blue-500" : "border-wi-line"}`}>
      <div className="flex items-center justify-between gap-3 bg-[var(--color-wi-row-alt)] px-3 py-2 border-b border-wi-line">
        <div className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider">
          CRM Row (snapshot {crmRow.snapshot_id.slice(0, 8)}&hellip;)
        </div>
        <button
          type="button"
          onClick={onSelect}
          aria-pressed={selected}
          className={`rounded-sm border px-2.5 py-1 text-xs font-semibold ${
            selected
              ? "border-blue-600 bg-blue-600 text-white"
              : "border-wi-line bg-white text-[var(--color-wi-text)] hover:border-blue-400"
          }`}
        >
          {selected ? "Selected" : `Select ${crmRow.course_name}`}
        </button>
      </div>
      <div className="p-3 space-y-2 text-sm">
        <div className="flex items-start gap-2">
          <span className="font-semibold text-[var(--color-wi-text-light)] w-24 shrink-0">Course:</span>
          <span>{crmRow.course_name}</span>
        </div>
        <div className="flex items-start gap-2">
          <span className="font-semibold text-[var(--color-wi-text-light)] w-24 shrink-0">Extra note:</span>
          <span className={crmRow.extra_note ? "font-mono text-amber-800" : "text-[var(--color-wi-text-light)] italic"}>
            {crmRow.extra_note || "(empty)"}
          </span>
        </div>
        {crmRow.imported_at && (
          <div className="flex items-start gap-2">
            <span className="font-semibold text-[var(--color-wi-text-light)] w-24 shrink-0">Imported:</span>
            <span className="text-[var(--color-wi-text-light)]">{new Date(crmRow.imported_at).toLocaleString()}</span>
          </div>
        )}
      </div>
    </div>
  );
}
