import { Check, Clock, X } from "lucide-react";

interface ProvisionalBadgeProps {
  studentOk: boolean;
  teacherOk: boolean;
  roomOk: boolean;
  onClick?: () => void;
}

function StatusIcon({ ok, pending }: { ok: boolean; pending?: boolean }) {
  if (ok) {
    return <Check className="inline w-3 h-3 text-green-600" aria-hidden="true" />;
  }
  if (pending) {
    return <Clock className="inline w-3 h-3 text-amber-500" aria-hidden="true" />;
  }
  return <X className="inline w-3 h-3 text-red-600" aria-hidden="true" />;
}

export function ProvisionalBadge({ studentOk, teacherOk, roomOk, onClick }: ProvisionalBadgeProps) {
  const allOk = studentOk && teacherOk && roomOk;

  const ariaLabel = allOk
    ? "Available"
    : `Provisional — Student ${studentOk ? "ok" : "conflict"}, Teacher ${teacherOk ? "ok" : "conflict"}, Room ${roomOk ? "ok" : "not set"}`;

  if (allOk) {
    return (
      <span
        role="status"
        aria-label={ariaLabel}
        onClick={onClick}
        className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-700 ${onClick ? "cursor-pointer hover:bg-green-200" : ""}`}
      >
        <Check className="w-3 h-3" aria-hidden="true" />
        Available
      </span>
    );
  }

  return (
    <span
      role="status"
      aria-label={ariaLabel}
      data-testid="checklist"
      onClick={onClick}
      className={`inline-flex items-center gap-2 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-800 ${onClick ? "cursor-pointer hover:bg-amber-200" : ""}`}
    >
      <Clock className="w-3 h-3 shrink-0" aria-hidden="true" />
      <span className="font-medium">Provisional</span>
      <span className="flex items-center gap-1.5 ml-1">
        <span className="inline-flex items-center gap-0.5">
          <StatusIcon ok={studentOk} />
          Student
        </span>
        <span className="inline-flex items-center gap-0.5">
          <StatusIcon ok={teacherOk} />
          Teacher
        </span>
        <span className="inline-flex items-center gap-0.5">
          <StatusIcon ok={roomOk} pending={!roomOk} />
          Room
        </span>
      </span>
    </span>
  );
}
