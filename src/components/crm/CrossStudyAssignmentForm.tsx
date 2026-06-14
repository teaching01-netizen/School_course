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

  const [sourceCourse, setSourceCourse] = useState(
    currentAssignment?.source_course?.id ?? crmRow?.course_id ?? "",
  );
  const [destA, setDestA] = useState(currentAssignment?.dest_course_a?.id ?? "");
  const [destB, setDestB] = useState(currentAssignment?.dest_course_b?.id ?? "");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setSourceCourse(currentAssignment?.source_course?.id ?? crmRow?.course_id ?? "");
    setDestA(currentAssignment?.dest_course_a?.id ?? "");
    setDestB(currentAssignment?.dest_course_b?.id ?? "");
  }, [crmRow?.course_id, currentAssignment]);

  const courseOptions = useMemo(
    () =>
      courses.map((c) => ({
        value: c.id,
        label: `${c.name}  ·  ${c.subject_name || "No subject"}`,
        keywords: `${c.code} ${c.name} ${c.subject_name}`,
      })),
    [courses],
  );

  const sourceCourseOption = useMemo(
    () => courses.find((c) => c.id === sourceCourse) ?? null,
    [courses, sourceCourse],
  );
  const courseA = useMemo(() => courses.find((c) => c.id === destA) ?? null, [courses, destA]);
  const courseB = useMemo(() => courses.find((c) => c.id === destB) ?? null, [courses, destB]);

  const canSave = sourceCourse && destA && destB;

  const handleSave = async () => {
    if (!canSave) return;
    setSaving(true);
    try {
      const snapshotId = crmRow?.snapshot_id || "";

      await apiJson("/api/v1/cross-study/assignments", {
        method: "PUT",
        body: JSON.stringify({
          wcode: student.wcode,
          source_course_id: sourceCourse,
          snapshot_id: snapshotId,
          dest_course_a_id: destA,
          dest_course_b_id: destB,
          assigned_course_id: destA,
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
      {/* Staff-owned source mapping */}
      <div className="bg-gray-50 rounded-sm p-3 space-y-3">
        <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">
          Staff assignment mapping
        </div>
        <div>
          <label className="block text-xs text-gray-500 mb-1">
            Source course to treat this row as
          </label>
          <TypeaheadSelect
            value={sourceCourse}
            onChange={setSourceCourse}
            options={courseOptions}
            placeholder="Search source course..."
          />
          <div className="mt-1 text-xs text-gray-400">
            CRM source text: {crmRow?.course_name || "No CRM source course"}
          </div>
          {!sourceCourse && (
            <div className="mt-2 rounded-sm border border-amber-200 bg-amber-50 px-2 py-1.5 text-xs text-amber-800">
              Choose the source course before saving. This mapping applies only to {student.wcode} and this CRM snapshot row.
            </div>
          )}
        </div>
      </div>

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
              onChange={setDestA}
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
              onChange={setDestB}
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

      {/* Assignment summary */}
      {courseA && courseB && (
        <div className="bg-gray-50 rounded-sm p-3 space-y-2">
          <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Assign student to both</div>
          <div className="flex items-center gap-2 p-2 rounded-sm bg-white">
            <span className="text-xs font-semibold text-gray-700">Source</span>
            <span className="text-sm">
              Treat row as: {sourceCourseOption?.name ?? "Choose source course"}
              {sourceCourseOption?.subject_name && (
                <span className="text-gray-400 ml-1">&middot; {sourceCourseOption.subject_name}</span>
              )}
            </span>
          </div>
          <div className="flex items-center gap-2 p-2 rounded-sm bg-white">
            <span className="text-xs font-semibold text-green-700">Included</span>
            <span className="text-sm">
              Course A: {courseA.name}
              <span className="text-gray-400 ml-1">&middot; {courseA.subject_name}</span>
            </span>
          </div>
          <div className="flex items-center gap-2 p-2 rounded-sm bg-white">
            <span className="text-xs font-semibold text-green-700">Included</span>
            <span className="text-sm">
              Course B: {courseB.name}
              <span className="text-gray-400 ml-1">&middot; {courseB.subject_name}</span>
            </span>
          </div>
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
