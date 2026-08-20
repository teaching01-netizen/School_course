import { useState } from "react";
import { Loader2, Pencil } from "lucide-react";
import { Popover } from "@/components/ui/Popover";
import { SubjectPicker } from "./SubjectPicker";
import type { Course, CourseEditChanges } from "../types";

interface CourseTitleProps {
  course: Course;
  /** The field currently being saved, or null. While any save is in flight
   *  the title is locked so two edits cannot race the optimistic-concurrency
   *  version bump. */
  savingField: string | null;
  onSave: (field: string, changes: CourseEditChanges) => Promise<boolean>;
}

/**
 * Notion-style page title. The course's identity is its subject, so the
 * heading shows the subject name (falling back to the course name for legacy
 * courses without one) and clicking it opens the searchable subject picker.
 */
export function CourseTitle({ course, savingField, onSave }: CourseTitleProps) {
  const [open, setOpen] = useState(false);
  const busy = savingField !== null;
  const savingSubject = savingField === "subject";
  const title = course.subject_name ?? course.name;

  return (
    <h1 className="mt-1">
      <Popover
        open={open}
        onOpenChange={setOpen}
        trigger={
          <button
            type="button"
            disabled={busy}
            aria-label="Edit subject"
            className="group inline-flex max-w-full items-center gap-2 rounded-sm px-1.5 py-1 text-left text-[32px] font-semibold tracking-[-0.01em] text-[var(--color-wi-text)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] disabled:cursor-not-allowed disabled:opacity-60 motion-reduce:transition-none"
          >
            <span className="truncate">{title}</span>
            {(savingSubject || busy) && (
              <Loader2 size={16} className="shrink-0 animate-spin text-[var(--color-wi-text-light)]" aria-hidden="true" />
            )}
            {!savingSubject && !busy && (
              <Pencil
                size={14}
                className="shrink-0 text-[var(--color-wi-faint)] opacity-0 transition-opacity duration-150 group-hover:opacity-100 motion-reduce:transition-none"
                aria-hidden="true"
              />
            )}
          </button>
        }
        ariaLabel="Edit subject"
        contentClassName="w-72"
      >
        <SubjectPicker course={course} saving={busy} onSave={onSave} close={() => setOpen(false)} />
      </Popover>
    </h1>
  );
}