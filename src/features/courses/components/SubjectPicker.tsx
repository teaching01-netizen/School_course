import { useEffect, useId, useRef, useState } from "react";
import TypeaheadSelect from "@/components/TypeaheadSelect";
import { getSubjects } from "../api/courseApi";
import type { Subject } from "@/types/shared";
import type { Course, CourseEditChanges } from "../types";

// The subject catalogue is small and shared by the title picker and the
// property-row picker; cache it module-wide so opening the second picker does
// not refetch. A failed fetch is retried on the next mount instead.
let cachedSubjects: Subject[] | null = null;
let subjectsInFlight: Promise<unknown> | null = null;

export function useSubjects(): { subjects: Subject[]; loading: boolean } {
  const [subjects, setSubjects] = useState<Subject[] | null>(cachedSubjects);

  useEffect(() => {
    if (cachedSubjects !== null || subjectsInFlight !== null) return;
    subjectsInFlight = getSubjects()
      .then((list) => {
        cachedSubjects = list;
        setSubjects(list);
      })
      .catch(() => setSubjects([]))
      .finally(() => {
        subjectsInFlight = null;
      });
  }, []);

  return { subjects: subjects ?? [], loading: subjects === null };
}

/**
 * The searchable subject picker shown inside the title and property-row
 * popovers. Picking a subject persists it immediately through onSave and
 * closes on success; on failure the popover stays open.
 */
export function SubjectPicker(props: {
  course: Course;
  saving: boolean;
  onSave: (field: string, changes: CourseEditChanges) => Promise<boolean>;
  close: () => void;
}) {
  const { course, saving, onSave, close } = props;
  const { subjects, loading } = useSubjects();
  const searchId = useId();
  // Set on Escape so the blur that fires when the focused input is removed
  // from the DOM (browsers fire blur on removal) cannot commit a cancelled
  // search as a save.
  const discardRef = useRef(false);

  const options = subjects.map((s) => ({ value: s.id, label: `${s.code} — ${s.name}` }));

  const pick = async (subjectId: string) => {
    if (discardRef.current) return;
    if (subjectId === course.subject_id) {
      close();
      return;
    }
    const ok = await onSave("subject", { subject_id: subjectId });
    if (ok) close();
  };

  return (
    <div className="w-full space-y-1 p-2" onKeyDownCapture={(e) => { if (e.key === "Escape") discardRef.current = true; }}>
      <p className="px-1 pt-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
        Subject
      </p>
      {loading ? (
        <p role="status" className="px-1 pb-2 text-[13px] text-[var(--color-wi-text-light)]">
          Loading…
        </p>
      ) : (
        <>
          <label htmlFor={searchId} className="sr-only">
            Search subject
          </label>
          <TypeaheadSelect
            id={searchId}
            value={course.subject_id ?? ""}
            onChange={(subjectId) => void pick(subjectId)}
            options={options}
            placeholder="Search subject…"
            disabled={saving}
          />
          {options.length === 0 && (
            <p role="status" className="px-1 pb-1 pt-1 text-[13px] text-[var(--color-wi-text-light)]">
              No subject available
            </p>
          )}
        </>
      )}
    </div>
  );
}