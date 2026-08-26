import type { CrmRowInfo } from "../../types/crossStudy";
import { Link2 } from "lucide-react";

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
        {crmRow.merge_group_name && (
          <div className="flex items-start gap-2" aria-label="Merge group context">
            <span className="w-24 shrink-0 font-semibold text-[var(--color-wi-text-light)]">Merged:</span>
            <span className="inline-flex min-w-0 items-start gap-1.5 rounded-sm border border-wi-line bg-[var(--color-wi-callout)] px-2 py-1 text-xs text-[var(--color-wi-text)]">
              <Link2
                aria-hidden="true"
                className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--color-wi-primary)]"
              />
              <span>
                <span className="font-semibold">{crmRow.merge_group_name}</span>
                {crmRow.merge_group_peer_course_name && (
                  <span className="block text-[var(--color-wi-text-light)]">
                    Paired with {crmRow.merge_group_peer_course_name}
                  </span>
                )}
              </span>
            </span>
          </div>
        )}
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
