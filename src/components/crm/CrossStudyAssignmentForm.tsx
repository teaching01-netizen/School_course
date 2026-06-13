import { useEffect, useMemo, useState } from "react";
import TypeaheadSelect from "../TypeaheadSelect";
import Button from "../ui/Button";
import CrossStudyStatusBadge from "./CrossStudyStatusBadge";
import type { AssignmentDTO, CrmRowInfo, StudentInfo } from "../../types/crossStudy";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";

type Props = {
  student: StudentInfo;
  crmRow: CrmRowInfo | null;
  currentAssignment: AssignmentDTO | null;
  courses: { id: string; code: string; name: string; subject_name: string }[];
  onSaved: () => void;
};

export default function CrossStudyAssignmentForm({ student, crmRow, currentAssignment, courses, onSaved }: Props) {
  const { addToast } = useToast();

  const [destA, setDestA] = useState(currentAssignment?.dest_course_a?.id ?? "");
  const [destB, setDestB] = useState(currentAssignment?.dest_course_b?.id ?? "");
  const [assigned, setAssigned] = useState(currentAssignment?.assigned_course_id ?? "");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setDestA(currentAssignment?.dest_course_a?.id ?? "");
    setDestB(currentAssignment?.dest_course_b?.id ?? "");
    setAssigned(currentAssignment?.assigned_course_id ?? "");
  }, [currentAssignment]);

  const courseOptions = useMemo(
    () =>
      courses.map((c) => ({
        value: c.id,
        label: `${c.name}  ·  ${c.subject_name || "No subject"}`,
        keywords: `${c.code} ${c.name} ${c.subject_name}`,
      })),
    [courses],
  );

  const courseA = useMemo(() => courses.find((c) => c.id === destA) ?? null, [courses, destA]);
  const courseB = useMemo(() => courses.find((c) => c.id === destB) ?? null, [courses, destB]);

  const canSave = destA && destB && assigned && assigned !== "";

  const handleSave = async () => {
    if (!canSave) return;
    setSaving(true);
    try {
      const sourceCourseId = crmRow?.course_id || currentAssignment?.dest_course_a?.id || "";
      const snapshotId = crmRow?.snapshot_id || "";

      await apiJson("/api/v1/cross-study/assignments", {
        method: "PUT",
        body: JSON.stringify({
          wcode: student.wcode,
          source_course_id: sourceCourseId,
          snapshot_id: snapshotId,
          dest_course_a_id: destA,
          dest_course_b_id: destB,
          assigned_course_id: assigned,
          extra_note_text: crmRow?.extra_note ?? "",
        }),
      });
      addToast("success", "Assignment saved");
      onSaved();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const handleRemove = async () => {
    if (!currentAssignment) return;
    setSaving(true);
    try {
      await apiJson(`/api/v1/cross-study/assignments/${currentAssignment.id}`, { method: "DELETE" });
      addToast("success", "Assignment removed");
      onSaved();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Remove failed");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* Course A/B selectors */}
      <div className="bg-gray-50 rounded-sm p-3 space-y-3">
        <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">
          Cross-study destination courses
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div>
            <label className="block text-xs text-gray-500 mb-1">Course A</label>
            <TypeaheadSelect
              value={destA}
              onChange={(v) => {
                setDestA(v);
                if (assigned && assigned !== v) setAssigned("");
              }}
              options={courseOptions}
              placeholder="Search course…"
            />
            {courseA && (
              <div className="mt-1 text-xs text-gray-400">
                {courseA.code} &middot; {courseA.subject_name || "No subject"}
              </div>
            )}
          </div>
          <div>
            <label className="block text-xs text-gray-500 mb-1">Course B</label>
            <TypeaheadSelect
              value={destB}
              onChange={(v) => {
                setDestB(v);
                if (assigned && assigned !== v) setAssigned("");
              }}
              options={courseOptions}
              placeholder="Search course…"
            />
            {courseB && (
              <div className="mt-1 text-xs text-gray-400">
                {courseB.code} &middot; {courseB.subject_name || "No subject"}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Radio selectors */}
      {courseA && courseB && (
        <div className="bg-gray-50 rounded-sm p-3 space-y-2">
          <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Assign student to</div>
          <label className="flex items-center gap-2 p-2 rounded-sm hover:bg-white cursor-pointer">
            <input type="radio" name="assigned" checked={assigned === destA} onChange={() => setAssigned(destA)} />
            <span className="text-sm">
              Course A: {courseA.name}
              <span className="text-gray-400 ml-1">&middot; {courseA.subject_name}</span>
            </span>
          </label>
          <label className="flex items-center gap-2 p-2 rounded-sm hover:bg-white cursor-pointer">
            <input type="radio" name="assigned" checked={assigned === destB} onChange={() => setAssigned(destB)} />
            <span className="text-sm">
              Course B: {courseB.name}
              <span className="text-gray-400 ml-1">&middot; {courseB.subject_name}</span>
            </span>
          </label>
        </div>
      )}

      {/* Status badge */}
      {currentAssignment && (
        <CrossStudyStatusBadge
          status={currentAssignment.status}
          extraNoteSnapshot={currentAssignment.extra_note_snapshot}
          currentNote={crmRow?.extra_note}
          sourceValid={currentAssignment.source_valid}
        />
      )}

      {/* Actions */}
      <div className="flex items-center gap-2">
        <Button variant="primary" size="md" loading={saving} disabled={!canSave || saving} onClick={handleSave}>
          {saving ? "Saving…" : "Save Assignment"}
        </Button>
        {currentAssignment && (
          <Button variant="danger" size="md" loading={saving} disabled={saving} onClick={handleRemove}>
            Remove Assignment
          </Button>
        )}
      </div>
    </div>
  );
}
