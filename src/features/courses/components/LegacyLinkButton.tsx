import { useState } from "react";
import { Link2 } from "lucide-react";
import Button from "@/components/ui/Button";
import { Popover } from "@/components/ui/Popover";
import { useToast } from "@/hooks/useToast";
import type { Course } from "../types";
import { extractLegacyCourseId, formatLegacySyncTime } from "../domain/legacyCourse";
import { syncLegacyCourse, updateCourse } from "../api/courseApi";

/**
 * Minimal legacy-system control: a single icon that opens a popover with the
 * full link/unlink/refresh management for the legacy course link. The resting
 * state carries no UI beyond the icon.
 */
export function LegacyLinkButton({ course, onLinked }: { course: Course; onLinked: () => void }) {
  const { addToast } = useToast();
  const [open, setOpen] = useState(false);
  const [urlInput, setUrlInput] = useState("");
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);

  const linked = course.legacy_course_id != null;

  const handleLink = async () => {
    const extracted = extractLegacyCourseId(urlInput);
    if (!extracted) {
      addToast("error", "Could not extract a numeric ID from the URL. Paste the full old system URL or just the numeric ID.");
      return;
    }
    try {
      setSaving(true);
      await updateCourse(course.id, { code: course.code, name: course.name, legacy_course_id: extracted });
      addToast("success", `Linked to old system ID ${extracted}`);
      setUrlInput("");
      setOpen(false);
      onLinked();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to link");
    } finally {
      setSaving(false);
    }
  };

  const handleUnlink = async () => {
    try {
      setSaving(true);
      await updateCourse(course.id, { code: course.code, name: course.name, legacy_course_id: null });
      addToast("success", "Removed legacy link");
      setOpen(false);
      onLinked();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to unlink");
    } finally {
      setSaving(false);
    }
  };

  const handleSyncNow = async () => {
    try {
      setSyncing(true);
      await syncLegacyCourse(course.id);
      addToast("success", "Legacy refresh queued");
      setOpen(false);
      onLinked();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Sync failed");
    } finally {
      setSyncing(false);
    }
  };

  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      align="end"
      role="dialog"
      ariaLabel="Legacy system link"
      trigger={
        <button
          type="button"
          aria-label={linked ? "Legacy system link" : "Link to old system"}
          title={linked ? `Linked to old system ID ${course.legacy_course_id}` : "Link to old system"}
          className={`inline-flex h-7 w-7 items-center justify-center rounded-sm transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] motion-reduce:transition-none ${
            linked ? "text-[var(--color-wi-text-light)]" : "text-[var(--color-wi-faint)]"
          }`}
        >
          <Link2 size={14} strokeWidth={2} aria-hidden="true" />
        </button>
      }
      contentClassName="w-72"
    >
      {linked ? (
        <div className="space-y-3 p-2">
          <div className="px-1 pt-1">
            <p className="text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Old System</p>
            <p className="mt-1 font-mono text-[13px] text-[var(--color-wi-text)]">ID: {course.legacy_course_id}</p>
            <p className="mt-0.5 text-[12px] text-[var(--color-wi-text-light)]">Last synced: {formatLegacySyncTime(course.legacy_last_synced_at)}</p>
          </div>
          <p className="px-1 text-[12px] text-[var(--color-wi-text-light)]">
            Managed by the legacy sync service. Local data remains available during source outages.
          </p>
          <div className="flex justify-end gap-2 pt-1">
            <Button variant="ghost" size="sm" onClick={() => void handleUnlink()} disabled={saving} className="text-[var(--color-wi-red)]">
              Remove link
            </Button>
            <Button variant="secondary" size="sm" onClick={() => void handleSyncNow()} disabled={syncing} loading={syncing}>
              {syncing ? "Queueing…" : "Queue refresh"}
            </Button>
          </div>
        </div>
      ) : (
        <div className="space-y-2 p-2">
          <p className="px-1 pt-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
            Link to Old System
          </p>
          <div className="flex items-center gap-2">
            <label htmlFor="legacy-link-input" className="sr-only">Legacy course ID or URL</label>
            <input
              id="legacy-link-input"
              type="text"
              value={urlInput}
              onChange={(e) => setUrlInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  void handleLink();
                }
              }}
              placeholder="Paste old system URL or numeric ID"
              className="min-w-0 flex-1 rounded-sm border border-wi-line px-2.5 py-1.5 text-sm transition-colors duration-150 placeholder:text-[var(--color-wi-faint)] focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15 focus-visible:outline-none"
            />
            <Button variant="primary" size="sm" onClick={() => void handleLink()} loading={saving} disabled={saving || !urlInput.trim()}>
              {saving ? "Linking…" : "Link"}
            </Button>
          </div>
        </div>
      )}
    </Popover>
  );
}
