type Props = {
  status: string;
  extraNoteSnapshot?: string;
  currentNote?: string;
  sourceValid?: boolean;
};

const statusConfig: Record<string, { icon: string; label: string; color: string }> = {
  active: { icon: "✅", label: "Active", color: "bg-green-50 text-green-700 border-green-200" },
  notes_changed: { icon: "⚠️", label: "Notes Changed", color: "bg-amber-50 text-amber-700 border-amber-200" },
  orphaned: { icon: "🔄", label: "Orphaned", color: "bg-red-50 text-red-700 border-red-200" },
  pending: { icon: "⏳", label: "Pending", color: "bg-blue-50 text-blue-700 border-blue-200" },
};

export default function CrossStudyStatusBadge({ status, extraNoteSnapshot, currentNote, sourceValid }: Props) {
  const cfg = statusConfig[status] ?? { icon: "❓", label: status, color: "bg-gray-50 text-gray-700 border-gray-200" };

  return (
    <div className={`rounded-sm border p-3 text-xs ${cfg.color}`}>
      <div className="font-semibold mb-1">
        {cfg.icon} Status: {cfg.label}
      </div>
      {status === "active" && (
        <div className="opacity-75">CRM course and extra note unchanged in the latest snapshot</div>
      )}
      {sourceValid === false && status !== "orphaned" && (
        <div className="mt-1">CRM row could not be verified in the latest snapshot.</div>
      )}
      {status === "notes_changed" && extraNoteSnapshot !== undefined && currentNote !== undefined && (
        <div className="space-y-0.5">
          <div>CRM course or extra note changed since last save:</div>
          <div className="font-mono">Was: &ldquo;{extraNoteSnapshot}&rdquo;</div>
          <div className="font-mono">Now: &ldquo;{currentNote}&rdquo;</div>
          <div className="mt-1">Review and re-save if assignment should change.</div>
        </div>
      )}
      {status === "orphaned" && (
        <div>
          <div>CRM row no longer appears in the latest snapshot.</div>
          <div className="mt-1">The student may have been moved to a different row or course.</div>
          <div>Review and re-assign or remove this assignment.</div>
        </div>
      )}
      {status === "pending" && (
        <div className="opacity-75">Assignment saved. Reconcile will pick it up on next import.</div>
      )}
    </div>
  );
}
