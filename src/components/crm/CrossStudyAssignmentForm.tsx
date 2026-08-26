import { useEffect, useMemo, useState } from "react";
import TypeaheadSelect from "../TypeaheadSelect";
import Button from "../ui/Button";
import CrossStudyStatusBadge from "./CrossStudyStatusBadge";
import type { AssignmentDTO, CrmRowInfo, StudentInfo } from "../../types/crossStudy";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";

type Props = {
  student: StudentInfo;
  crmRow: CrmRowInfo;
  currentAssignment: AssignmentDTO | null;
  courses: { id: string; code: string; name: string; subject_name: string }[];
  onSaved: () => void;
};

const weekdays = [
  { value: 1, label: "Mon" },
  { value: 2, label: "Tue" },
  { value: 3, label: "Wed" },
  { value: 4, label: "Thu" },
  { value: 5, label: "Fri" },
  { value: 6, label: "Sat" },
  { value: 7, label: "Sun" },
];

function toggleWeekday(values: number[], day: number) {
  return values.includes(day)
    ? values.filter((value) => value !== day)
    : [...values, day].sort((a, b) => a - b);
}

function WeekdaySelector({
  label,
  values,
  onChange,
}: {
  label: "Course A" | "Course B";
  values: number[];
  onChange: (values: number[]) => void;
}) {
  return (
    <fieldset className="mt-2">
      <legend className="mb-1 text-xs font-medium text-[var(--color-wi-text-light)]">Attend sessions</legend>
      <div className="grid grid-cols-4 gap-1 sm:grid-cols-7">
        {weekdays.map((day) => {
          const checked = values.includes(day.value);
          return (
            <label
              key={day.value}
              className={`flex min-h-10 cursor-pointer items-center justify-center rounded-sm border px-2 text-xs font-medium ${
                checked
                  ? "border-blue-500 bg-blue-50 text-blue-700"
                  : "border-wi-line bg-white text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)]"
              }`}
            >
              <input
                type="checkbox"
                className="sr-only"
                checked={checked}
                aria-label={`${label} ${day.label}`}
                onChange={() => onChange(toggleWeekday(values, day.value))}
              />
              {day.label}
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}

export default function CrossStudyAssignmentForm({ student, crmRow, currentAssignment, courses, onSaved }: Props) {
  const { addToast } = useToast();

  const [destA, setDestA] = useState(currentAssignment?.dest_course_a?.id ?? "");
  const [destB, setDestB] = useState(currentAssignment?.dest_course_b?.id ?? "");
  const [destAWeekdays, setDestAWeekdays] = useState(currentAssignment?.dest_course_a_weekdays ?? []);
  const [destBWeekdays, setDestBWeekdays] = useState(currentAssignment?.dest_course_b_weekdays ?? []);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setDestA(currentAssignment?.dest_course_a?.id ?? "");
    setDestB(currentAssignment?.dest_course_b?.id ?? "");
    setDestAWeekdays(currentAssignment?.dest_course_a_weekdays ?? []);
    setDestBWeekdays(currentAssignment?.dest_course_b_weekdays ?? []);
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

  const canSave = destA && destB && destAWeekdays.length > 0 && destBWeekdays.length > 0;

  const handleSave = async () => {
    if (!canSave) return;
    setSaving(true);
    try {
      await apiJson("/api/v1/cross-study/assignments", {
        method: "PUT",
        body: JSON.stringify({
          wcode: student.wcode,
          snapshot_id: crmRow.snapshot_id,
          crm_course_name: crmRow.course_name,
          crm_row_hash: crmRow.row_hash,
          crm_xlsx_row_number: crmRow.xlsx_row_number,
          dest_course_a_id: destA,
          dest_course_b_id: destB,
          dest_course_a_weekdays: destAWeekdays,
          dest_course_b_weekdays: destBWeekdays,
          assigned_course_id: destA,
          extra_note_text: crmRow.extra_note,
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
      <div className="bg-[var(--color-wi-row-alt)] rounded-sm p-3 space-y-3">
        <div className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider">
          Cross-study destination courses
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div>
            <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Course A</label>
            <TypeaheadSelect
              value={destA}
              onChange={setDestA}
              options={courseOptions}
              placeholder="Search course…"
            />
            {courseA && (
              <div className="mt-1 text-xs text-[var(--color-wi-text-light)]">
                {courseA.code} &middot; {courseA.subject_name || "No subject"}
              </div>
            )}
            {destA && (
              <WeekdaySelector label="Course A" values={destAWeekdays} onChange={setDestAWeekdays} />
            )}
          </div>
          <div>
            <label className="block text-xs text-[var(--color-wi-text-light)] mb-1">Course B</label>
            <TypeaheadSelect
              value={destB}
              onChange={setDestB}
              options={courseOptions}
              placeholder="Search course…"
            />
            {courseB && (
              <div className="mt-1 text-xs text-[var(--color-wi-text-light)]">
                {courseB.code} &middot; {courseB.subject_name || "No subject"}
              </div>
            )}
            {destB && (
              <WeekdaySelector label="Course B" values={destBWeekdays} onChange={setDestBWeekdays} />
            )}
          </div>
        </div>
        {destA && destB && (!destAWeekdays.length || !destBWeekdays.length) && (
          <div className="rounded-sm border border-amber-200 bg-amber-50 px-2 py-1.5 text-xs text-amber-800">
            Choose at least one attendance day for each destination course before saving.
          </div>
        )}
      </div>

      {/* Assignment summary */}
      {courseA && courseB && (
        <div className="bg-[var(--color-wi-row-alt)] rounded-sm p-3 space-y-2">
          <div className="text-xs font-semibold text-[var(--color-wi-text-light)] uppercase tracking-wider">Assign student to both</div>
          <div className="flex items-center gap-2 p-2 rounded-sm bg-white">
            <span className="text-xs font-semibold text-green-700">Included</span>
            <span className="text-sm">
              Course A: {courseA.name}
              <span className="text-[var(--color-wi-text-light)] ml-1">
                ({weekdays.filter((day) => destAWeekdays.includes(day.value)).map((day) => day.label).join(", ") || "choose days"})
              </span>
              <span className="text-[var(--color-wi-text-light)] ml-1">&middot; {courseA.subject_name}</span>
            </span>
          </div>
          <div className="flex items-center gap-2 p-2 rounded-sm bg-white">
            <span className="text-xs font-semibold text-green-700">Included</span>
            <span className="text-sm">
              Course B: {courseB.name}
              <span className="text-[var(--color-wi-text-light)] ml-1">
                ({weekdays.filter((day) => destBWeekdays.includes(day.value)).map((day) => day.label).join(", ") || "choose days"})
              </span>
              <span className="text-[var(--color-wi-text-light)] ml-1">&middot; {courseB.subject_name}</span>
            </span>
          </div>
        </div>
      )}

      {/* Status badge */}
      {currentAssignment && (
        <CrossStudyStatusBadge
          status={currentAssignment.status}
          extraNoteSnapshot={currentAssignment.extra_note_snapshot}
          currentNote={crmRow.extra_note}
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
