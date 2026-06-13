import type { CrmRowInfo } from "../../types/crossStudy";

type Props = {
  crmRow: CrmRowInfo;
};

export default function CrossStudyCrmRowCard({ crmRow }: Props) {
  return (
    <div className="border border-gray-200 rounded-sm overflow-hidden">
      <div className="bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-500 uppercase tracking-wider border-b border-gray-200">
        CRM Row (snapshot {crmRow.snapshot_id.slice(0, 8)}&hellip;)
      </div>
      <div className="p-3 space-y-2 text-sm">
        <div className="flex items-start gap-2">
          <span className="font-semibold text-gray-600 w-24 shrink-0">Course:</span>
          <span>{crmRow.course_name}</span>
        </div>
        <div className="flex items-start gap-2">
          <span className="font-semibold text-gray-600 w-24 shrink-0">Extra note:</span>
          <span className={crmRow.extra_note ? "font-mono text-amber-800" : "text-gray-400 italic"}>
            {crmRow.extra_note || "(empty)"}
          </span>
        </div>
        {crmRow.imported_at && (
          <div className="flex items-start gap-2">
            <span className="font-semibold text-gray-600 w-24 shrink-0">Imported:</span>
            <span className="text-gray-500">{new Date(crmRow.imported_at).toLocaleString()}</span>
          </div>
        )}
      </div>
    </div>
  );
}
