import type { ReactNode } from "react";
import { Inbox } from "lucide-react";

interface EmptyStateProps {
  message: string;
  icon?: ReactNode;
  action?: ReactNode;
}

export default function EmptyState({ message, icon, action }: EmptyStateProps) {
  return (
    <div className="py-12 text-center">
      <div className="flex justify-center mb-2 text-[var(--color-wi-faint)]">
        {icon ?? <Inbox className="w-10 h-10" aria-hidden="true" />}
      </div>
      <p className="text-[var(--color-wi-text-light)] text-sm mb-3">{message}</p>
      {action && <div>{action}</div>}
    </div>
  );
}
