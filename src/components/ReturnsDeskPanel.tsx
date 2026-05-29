import { useState, useCallback } from "react";
import { RotateCcw, Trash2 } from "lucide-react";
import SlideOver from "./SlideOver";
import Button from "./ui/Button";
import type { ReturnsDeskEntry } from "../hooks/useReturnsDesk";

export type ReturnsDeskPanelProps = {
  isOpen: boolean;
  onClose: () => void;
  entries: Record<string, ReturnsDeskEntry[]>;
  onRetry: (entry: ReturnsDeskEntry) => Promise<boolean>;
  onDismiss: (id: string) => void;
  totalCount: number;
};

const LABELS: Record<string, string> = {
  stale_edit: "Stale Edit",
  duplicate_level: "Duplicate Level",
};

function entryLabel(code: string): string {
  return LABELS[code] ?? code;
}

function EntryCard({
  entry,
  onRetry,
  onDismiss,
}: {
  entry: ReturnsDeskEntry;
  onRetry: (entry: ReturnsDeskEntry) => Promise<boolean>;
  onDismiss: (id: string) => void;
}) {
  const [retrying, setRetrying] = useState(false);

  const handleRetry = useCallback(async () => {
    setRetrying(true);
    try {
      await onRetry(entry);
    } finally {
      setRetrying(false);
    }
  }, [entry, onRetry]);

  return (
    <div className="border border-gray-200 rounded-sm p-3 mb-2 bg-white">
      <div className="flex items-start justify-between mb-2">
        <div>
          <span className="font-mono text-sm font-semibold text-[var(--color-wi-text)]">
            {entry.courseCode}
          </span>
          <span className="ml-2 text-xs text-gray-500">
            Level: {entry.attemptedLevel ?? "Not set"}
          </span>
        </div>
      </div>
      <p className="text-sm text-gray-600 mb-3">{entry.error.message}</p>
      <div className="flex gap-2">
        <Button
          variant="primary"
          size="sm"
          loading={retrying}
          onClick={handleRetry}
        >
          <RotateCcw className="w-3 h-3 mr-1" />
          Retry
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onDismiss(entry.id)}
        >
          <Trash2 className="w-3 h-3 mr-1" />
          Dismiss
        </Button>
      </div>
    </div>
  );
}

export default function ReturnsDeskPanel({
  isOpen: _isOpen,
  onClose,
  entries,
  onRetry,
  onDismiss,
  totalCount,
}: ReturnsDeskPanelProps) {
  const [panelOpen, setPanelOpen] = useState(false);
  const codes = Object.keys(entries);

  const handleClearAll = useCallback(() => {
    for (const code of codes) {
      for (const entry of entries[code]) {
        onDismiss(entry.id);
      }
    }
  }, [codes, entries, onDismiss]);

  return (
    <>
      {totalCount > 0 && (
        <button
          onClick={() => setPanelOpen(true)}
          className="fixed top-4 right-20 z-40 bg-red-500 text-white rounded-full px-2 py-0.5 text-xs font-bold shadow-lg hover:bg-red-600 transition-colors"
          aria-label={`${totalCount} failed operations`}
        >
          {totalCount}
        </button>
      )}

      {panelOpen && (
        <SlideOver
          title="Returns Desk"
          onClose={() => {
            setPanelOpen(false);
            onClose();
          }}
          footer={
            <Button
              variant="ghost"
              size="sm"
              onClick={handleClearAll}
              disabled={totalCount === 0}
            >
              Clear All
            </Button>
          }
        >
          {totalCount === 0 ? (
            <p className="text-sm text-gray-500 text-center py-8">
              No failed operations
            </p>
          ) : (
            codes.map((code) => (
              <div key={code} className="mb-4">
                <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500 mb-2">
                  {entryLabel(code)}
                </h4>
                {entries[code].map((entry) => (
                  <EntryCard
                    key={entry.id}
                    entry={entry}
                    onRetry={onRetry}
                    onDismiss={onDismiss}
                  />
                ))}
              </div>
            ))
          )}
        </SlideOver>
      )}
    </>
  );
}
