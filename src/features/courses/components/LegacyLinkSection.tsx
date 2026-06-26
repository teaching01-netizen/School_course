import { useState } from "react";
import Button from "@/components/ui/Button";
import { useToast } from "@/hooks/useToast";
import type { Course } from "../types";
import { extractLegacyCourseId } from "../domain/legacyCourse";
import { syncLegacyCourse, updateCourse } from "../api/courseApi";

export function LegacyLinkSection({ course, onLinked }: { course: Course; onLinked: () => void }) {
  const { addToast } = useToast();
  const [urlInput, setUrlInput] = useState("");
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);

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
      addToast("success", "Legacy sync completed");
      onLinked();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Sync failed");
    } finally {
      setSyncing(false);
    }
  };

  if (course.legacy_course_id) {
    return (
      <div className="mb-4 p-3 bg-gray-50 border border-gray-200 rounded-sm text-xs">
        <div className="flex items-center gap-2">
          <span className="font-semibold text-gray-700">Old System</span>
          <span className="text-gray-500">ID: {course.legacy_course_id}</span>
          <a
            href={`https://warwick.azurewebsites.net/Admin/Courses/Detail?id=${course.legacy_course_id}`}
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-600 underline"
          >
            Open in old system
          </a>
          <Button variant="secondary" size="sm" loading={syncing} onClick={handleSyncNow} disabled={syncing} className="ml-auto">
            {syncing ? "Syncing..." : "Sync now"}
          </Button>
          <Button variant="ghost" size="sm" onClick={handleUnlink} disabled={saving} className="text-red-600">
            Remove link
          </Button>
        </div>
        {course.legacy_last_synced_at && (
          <div className="text-gray-400 mt-1">
            Last synced: {course.legacy_last_synced_at}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="mb-4 p-3 bg-gray-50 border border-gray-200 rounded-sm text-xs">
      <div className="font-semibold text-gray-700 mb-2">Link to Old System</div>
      <div className="flex items-center gap-2">
        <input
          type="text"
          value={urlInput}
          onChange={(e) => setUrlInput(e.target.value)}
          placeholder="Paste old system URL or numeric ID"
          className="flex-1 px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
        />
        <Button variant="primary" size="sm" loading={saving} onClick={handleLink} disabled={saving || !urlInput.trim()}>
          {saving ? "Linking..." : "Link"}
        </Button>
      </div>
    </div>
  );
}
