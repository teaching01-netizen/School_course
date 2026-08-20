import type { CrmRowInfo } from "../../types/crossStudy";

type Props = {
  crmRow: CrmRowInfo;
};

export default function CrossStudyCrmRowCard({ crmRow }: Props) {
  return (
    <div className="border border-[var(--color-wi-line)] rounded-sm overflow-hidden">
      <div className="bg-[var(--color-wi-row-alt)] px-3 py-2 text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider border-b border-b-[var(--color-wi-line)]">
        CRM Row (snapshot {crmRow.snapshot_id.slice(0, 8)}&hellip;)
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
