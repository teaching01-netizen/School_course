import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Check } from "lucide-react";
import { Popover } from "@/components/ui/Popover";
import Button from "@/components/ui/Button";
import { Switch } from "@/components/ui/Switch";
import { Tooltip } from "@/components/ui/Tooltip";
import type { TypeaheadOption } from "@/components/TypeaheadSelect";
import { CourseTeacherEditor } from "./CourseTeacherEditor";
import { SubjectPicker } from "./SubjectPicker";
import type { Course, CourseEditChanges, CourseTeacher, EditableTeacher } from "../types";
import type { CourseCycle } from "../api/courseApi";

const TEACHER_ICON = (
  <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
    <path d="M6 5.5a2.25 2.25 0 1 0 0-4.5 2.25 2.25 0 0 0 0 4.5Zm-3.25 4a3.25 3.25 0 0 1 6.5 0" stroke="currentColor" strokeWidth="1.5" fill="none" strokeLinecap="round" />
  </svg>
);

/** Shared two-column property row: fixed label column, filling value column —
 *  the same grammar Notion uses for its page property list. */
const ROW_GRID = "grid grid-cols-[minmax(0,6rem)_minmax(0,1fr)] items-center gap-x-2";

interface CoursePropertiesPanelProps {
  course: Course;
  teacherOptions: TypeaheadOption[];
  teacherNameById?: Map<string, string>;
  cycles?: CourseCycle[];
  /** The field currently being saved, or null. While any save is in flight
   *  every row is locked so two edits cannot race the version bump. */
  savingField: string | null;
  onSave: (field: string, changes: CourseEditChanges) => Promise<boolean>;
}

export function CoursePropertiesPanel({ course, teacherOptions, teacherNameById, cycles = [], savingField, onSave }: CoursePropertiesPanelProps) {
  const busy = savingField !== null;

  const typeOptions = useMemo(
    () => [
      { id: "Private", label: "Private" },
      { id: "Group", label: "Group" },
    ],
    [],
  );

  const pickType = async (id: string, close: () => void) => {
    const ok = await onSave("course_type", { course_type: id });
    if (ok) close();
  };

  return (
    <div className="w-full">
      <p className="px-1.5 pb-1.5 pt-0.5 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
        Properties
      </p>
      <div className="space-y-0.5">
        <div className={`${ROW_GRID} rounded-[4px] px-1.5 py-1`}>
          <span className="text-[13px] text-[var(--color-wi-text-light)]">Code</span>
          <span className="truncate font-mono text-[13px] text-[var(--color-wi-text)]">{course.code}</span>
        </div>

        <PropertyRow
          label="Name"
          saving={busy}
          contentClassName="w-56"
          value={course.name ? <span className="min-w-0 truncate">{course.name}</span> : null}
          placeholder="None"
          editor={(close) => <TextEditor field="name" label="Name" value={course.name} saving={busy} onSave={onSave} close={close} />}
        />

        <PropertyRow
          label="Teachers"
          saving={busy}
          contentClassName="w-80"
          value={<TeacherChips teachers={course.teachers} primaryId={course.primary_teacher_id} teacherNameById={teacherNameById} fallbackName={course.teacher_name} />}
          placeholder="No teachers"
          editor={(close) => <TeachersEditor course={course} teacherOptions={teacherOptions} saving={busy} onSave={onSave} close={close} />}
        />

        <PropertyRow
          label="Subject"
          saving={busy}
          contentClassName="w-72"
          value={course.subject_name ? <span className="min-w-0 truncate">{course.subject_name}</span> : null}
          placeholder="No subject"
          editor={(close) => <SubjectPicker course={course} saving={busy} onSave={onSave} close={close} />}
        />

        <PropertyRow
          label="Type"
          saving={busy}
          contentClassName="w-max min-w-56 max-w-[min(28rem,calc(100vw-2rem))]"
          value={course.course_type ? <span className="min-w-0 truncate">{course.course_type}</span> : null}
          placeholder="No type"
          editor={(close) => (
            <ChoiceEditor
              ariaLabel="Course type"
              options={typeOptions}
              selectedId={course.course_type ?? null}
              disabled={busy}
              onPick={(id) => void pickType(id, close)}
            />
          )}
        />

        <PropertyRow
          label="Year"
          saving={busy}
          contentClassName="w-56"
          value={course.year != null ? <span className="min-w-0 truncate">{course.year}</span> : null}
          placeholder="None"
          editor={(close) => <NumberEditor field="year" label="Year" value={course.year} saving={busy} onSave={onSave} close={close} />}
        />

        <PropertyRow
          label="Hour"
          saving={busy}
          contentClassName="w-56"
          value={course.hour != null ? <span className="min-w-0 truncate">{course.hour}</span> : null}
          placeholder="None"
          editor={(close) => <NumberEditor field="hour" label="Hour" value={course.hour} saving={busy} onSave={onSave} close={close} />}
        />

        <PropertyRow
          label="Students"
          saving={busy}
          contentClassName="w-56"
          value={course.student_count != null ? <span className="min-w-0 truncate">{course.student_count}</span> : null}
          placeholder="None"
          editor={(close) => <NumberEditor field="student_count" label="Students" value={course.student_count} saving={busy} onSave={onSave} close={close} />}
        />

        <PropertyRow
          label="Cycle"
          saving={busy}
          contentClassName="w-max min-w-72 max-w-[min(28rem,calc(100vw-2rem))]"
          value={course.cycle_label ? <span className="min-w-0 truncate">{course.cycle_label}</span> : null}
          placeholder="No cycle"
          editor={(close) => (
            <ChoiceEditor
              ariaLabel="Course cycle"
              options={[{ id: "__none__", label: "No cycle" }, ...cycles.map((cycle) => ({ id: cycle.id, label: cycle.display_name ?? cycle.label }))]}
              selectedId={course.cycle_id ?? "__none__"}
              disabled={busy}
              onPick={(id) => void onSave("cycle_id", { cycle_id: id === "__none__" ? null : id }).then((ok) => { if (ok) close(); })}
            />
          )}
        />

        <PropertyRow
          label="Expires"
          saving={busy}
          contentClassName="w-56"
          value={course.expiry_days == null ? null : <span className="min-w-0 truncate">{course.expiry_days} days</span>}
          placeholder="No expiration"
          editor={(close) => <ExpiryEditor value={course.expiry_days} saving={busy} onSave={onSave} close={close} />}
        />

        <AbsenceFormRow course={course} busy={busy} onSave={onSave} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Absence form visibility — inline toggle
// ---------------------------------------------------------------------------

/** The only student-facing property in the panel: whether students can select
 *  this class in the absence form. A boolean gets a switch, not a picker — one
 *  click, state + consequence always visible, no popover round-trip. The
 *  switch is fully derived from the course (server truth): while a save is in
 *  flight the row locks, so the toggle can never drift from what was saved. */
function AbsenceFormRow({ course, busy, onSave }: { course: Course; busy: boolean; onSave: (field: string, changes: CourseEditChanges) => Promise<boolean> }) {
  const visible = course.absence_form_visible !== false;
  return (
    <div className="group grid grid-cols-[minmax(0,6rem)_minmax(0,1fr)] items-center gap-x-2 rounded-[4px] px-1.5 py-1 transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] motion-reduce:transition-none">
      <span className="flex items-center gap-1 text-[13px] text-[var(--color-wi-text-light)]">
        <span className="truncate">Absence form</span>
        <Tooltip content="Controls the student absence form only. Students cannot see or book hidden classes; staff always can." />
      </span>
      <span className="flex min-w-0 items-center gap-2">
        <Switch
          checked={visible}
          onCheckedChange={(next) => void onSave("absence_form_visible", { absence_form_visible: next })}
          disabled={busy}
          aria-label={`Show ${course.code} in the student absence form`}
        />
        {visible ? (
          <span className="min-w-0 truncate text-[13px] text-[var(--color-wi-text-light)]">Shown to students</span>
        ) : (
          <span className="min-w-0 truncate rounded-sm bg-amber-100 px-1.5 py-px text-[11px] font-semibold uppercase tracking-wide text-amber-800">
            Hidden from students
          </span>
        )}
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Row + anchored popover
// ---------------------------------------------------------------------------

function PropertyRow(props: {
  label: string;
  saving: boolean;
  value: ReactNode;
  placeholder?: string;
  contentClassName?: string;
  editor: (close: () => void) => ReactNode;
}) {
  const { label, saving, value, placeholder, contentClassName, editor } = props;
  const [open, setOpen] = useState(false);

  return (
    <div className="group grid grid-cols-[minmax(0,6rem)_minmax(0,1fr)] items-center gap-x-2 rounded-[4px] px-1.5 py-1 transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] motion-reduce:transition-none">
      <span className="truncate text-[13px] text-[var(--color-wi-text-light)]">{label}</span>
      <span className="flex min-w-0 items-center">
        <Popover
          open={open}
          onOpenChange={setOpen}
          trigger={
            <button
              type="button"
              disabled={saving}
              className="flex min-h-6 min-w-0 cursor-pointer items-center text-start text-[13px] text-[var(--color-wi-text)] transition-colors duration-150 focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] disabled:cursor-not-allowed disabled:opacity-60 motion-reduce:transition-none"
            >
              {value ?? <span className="text-[var(--color-wi-faint)]">{placeholder}</span>}
            </button>
          }
          ariaLabel={`Edit ${label}`}
          contentClassName={contentClassName}
        >
          {editor(() => setOpen(false))}
        </Popover>
      </span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Value rendering
// ---------------------------------------------------------------------------

function TeacherChips({
  teachers,
  primaryId,
  teacherNameById,
  fallbackName,
}: {
  teachers?: CourseTeacher[];
  primaryId: string | null | undefined;
  teacherNameById?: Map<string, string>;
  fallbackName?: string | null;
}) {
  if (teachers?.length) {
    return (
      <span className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5">
        {teachers.map((teacher) => (
          <span key={teacher.id} className="inline-flex min-w-0 items-center gap-1">
            {TEACHER_ICON}
            <span className="truncate">{teacherNameById?.get(teacher.id) ?? teacher.full_name ?? teacher.username}</span>
            {primaryId === teacher.id && (
              <span className="rounded-sm bg-[var(--color-wi-row-alt)] px-1 py-px text-[10px] font-semibold uppercase text-[var(--color-wi-text-light)]">
                Primary
              </span>
            )}
          </span>
        ))}
      </span>
    );
  }
  if (fallbackName) {
    return (
      <span className="inline-flex min-w-0 items-center gap-1">
        {TEACHER_ICON}
        <span className="truncate">{fallbackName}</span>
      </span>
    );
  }
  return null;
}

// ---------------------------------------------------------------------------
// Editors
// ---------------------------------------------------------------------------

function TeachersEditor(props: {
  course: Course;
  teacherOptions: TypeaheadOption[];
  saving: boolean;
  onSave: (field: string, changes: CourseEditChanges) => Promise<boolean>;
  close: () => void;
}) {
  const { course, teacherOptions, saving, onSave, close } = props;
  const [draft, setDraft] = useState<EditableTeacher[]>(() =>
    (course.teachers ?? []).map((t) => ({ teacher_id: t.id, is_primary: t.is_primary })),
  );
  const [submitting, setSubmitting] = useState(false);

  // Re-seed from the latest course (stale_edit reload) while the popover stays
  // open, so the editor never shows a stale teacher set after a conflict.
  useEffect(() => {
    setDraft((course.teachers ?? []).map((t) => ({ teacher_id: t.id, is_primary: t.is_primary })));
  }, [course]);

  const handleSave = async () => {
    if (submitting || saving) return;
    setSubmitting(true);
    const ok = await onSave("teachers", { teachers: draft });
    setSubmitting(false);
    if (ok) close();
  };

  return (
    <div className="w-full space-y-3 p-2">
      <p className="px-1 pt-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
        Teachers
      </p>
      <CourseTeacherEditor teachers={draft} onChange={setDraft} options={teacherOptions} disabled={submitting || saving} />
      <div className="flex justify-end gap-2 pt-1">
        <Button variant="secondary" size="sm" onMouseDown={(e) => e.preventDefault()} onClick={close}>
          Cancel
        </Button>
        <Button variant="primary" size="sm" onClick={() => void handleSave()} loading={submitting || saving}>
          Save
        </Button>
      </div>
    </div>
  );
}

function ChoiceEditor(props: {
  ariaLabel: string;
  options: { id: string; label: string }[];
  loading?: boolean;
  selectedId: string | null;
  disabled: boolean;
  onPick: (id: string) => void;
}) {
  const { ariaLabel, options, loading = false, selectedId, disabled, onPick } = props;
  const [query, setQuery] = useState("");
  const filteredOptions = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return options;
    return options.filter((option) => option.label.toLowerCase().includes(needle));
  }, [options, query]);

  if (loading) {
    return (
      <p role="status" className="w-full px-3 py-3 text-center text-[13px] text-[var(--color-wi-text-light)]">
        Loading…
      </p>
    );
  }
  if (options.length === 0) {
    return (
      <p role="status" className="w-full px-3 py-3 text-center text-[13px] text-[var(--color-wi-text-light)]">
        No options available
      </p>
    );
  }
  return (
    <div role="listbox" aria-label={ariaLabel} className="w-full p-1.5">
      <input
        role="combobox"
        aria-label={`Search ${ariaLabel}`}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="Search options…"
        className="mb-1.5 h-8 w-full rounded-sm border border-wi-line px-2.5 text-xs placeholder:text-[var(--color-wi-faint)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-3 focus:ring-[var(--color-wi-primary)]/15"
      />
      <div className="notion-scrollbar max-h-52 overflow-y-auto">
      {filteredOptions.length === 0 ? <div className="px-2.5 py-3 text-center text-xs text-[var(--color-wi-text-light)]">No matches found</div> : null}
      {filteredOptions.map((option) => {
        const selected = option.id === selectedId;
        return (
          <button
            key={option.id}
            type="button"
            role="option"
            aria-selected={selected}
            disabled={disabled}
            onClick={() => onPick(option.id)}
            className={`flex min-h-9 w-full items-center justify-between gap-2 rounded-sm px-2.5 py-1.5 text-left text-sm transition-[background-color,transform] duration-150 focus-visible:outline-none active:translate-y-px disabled:cursor-not-allowed disabled:opacity-60 motion-reduce:transition-none ${
              selected ? "bg-[var(--color-wi-row-alt)] font-medium text-[var(--color-wi-text)]" : "text-[var(--color-wi-text)] hover:bg-[var(--color-wi-row-alt)]"
            }`}
          >
            <span className="min-w-0 flex-1 break-words">{option.label}</span>
            {selected && <Check size={14} strokeWidth={2.5} className="shrink-0 text-[var(--color-wi-primary)]" aria-hidden="true" />}
          </button>
        );
      })}
      </div>
    </div>
  );
}

function TextEditor(props: {
  field: "name";
  label: string;
  value: string | null | undefined;
  saving: boolean;
  onSave: (field: string, changes: CourseEditChanges) => Promise<boolean>;
  close: () => void;
}) {
  const { field, label, value, saving, onSave, close } = props;
  const [draft, setDraft] = useState(value ?? "");
  const [submitting, setSubmitting] = useState(false);
  const committingRef = { current: false };
  // Set by Escape so the blur that fires when the focused input is removed
  // from the DOM (browsers fire blur on removal) cannot commit a cancelled
  // edit as a save.
  const discardRef = { current: false };

  const commit = async () => {
    if (committingRef.current || submitting || saving) return;
    if (discardRef.current) {
      discardRef.current = false;
      close();
      return;
    }
    const next = draft.trim();
    if (!next) return;
    if (next === value) {
      close();
      return;
    }
    committingRef.current = true;
    setSubmitting(true);
    const ok = await onSave(field, { [field]: next });
    setSubmitting(false);
    committingRef.current = false;
    if (ok) close();
  };

  return (
    <div className="w-full space-y-3 p-2">
      <p className="px-1 pt-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
        {label}
      </p>
      <input
        autoFocus
        type="text"
        aria-label={label}
        value={draft}
        onChange={(e) => {
          discardRef.current = false;
          setDraft(e.target.value);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            void commit();
          } else if (e.key === "Escape") {
            e.preventDefault();
            discardRef.current = true;
            setDraft(value ?? "");
            close();
          }
        }}
        onBlur={() => {
          if (!committingRef.current) void commit();
        }}
        disabled={submitting || saving}
        className="h-8 w-full rounded-sm border border-wi-line bg-white px-2.5 text-sm text-[var(--color-wi-text)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15 disabled:opacity-60"
      />
      <div className="flex justify-end gap-2 pt-1">
        <Button variant="secondary" size="sm" onMouseDown={(e) => e.preventDefault()} onClick={close}>
          Cancel
        </Button>
        <Button variant="primary" size="sm" onClick={() => void commit()} loading={submitting || saving}>
          Save
        </Button>
      </div>
    </div>
  );
}

function ExpiryEditor({ value, saving, onSave, close }: { value: number | null | undefined; saving: boolean; onSave: (field: string, changes: CourseEditChanges) => Promise<boolean>; close: () => void }) {
  const [draft, setDraft] = useState(value == null ? "" : String(value));
  const [submitting, setSubmitting] = useState(false);
  const commit = async (next: number | null) => {
    if (submitting || saving) return;
    setSubmitting(true);
    const ok = await onSave("expiry_days", { expiry_days: next });
    setSubmitting(false);
    if (ok) close();
  };
  return (
    <div className="w-full space-y-2 p-2">
      <p className="px-1 pt-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Expiration days</p>
      <input autoFocus type="number" min="0" step="1" aria-label="Expiration days" value={draft} onChange={(e) => setDraft(e.target.value)} className="h-8 w-full rounded-sm border border-wi-line bg-white px-2.5 text-sm" />
      <div className="flex justify-between gap-2">
        <Button variant="secondary" size="sm" disabled={submitting || saving} onClick={() => void commit(null)}>No expiration</Button>
        <Button variant="primary" size="sm" loading={submitting || saving} onClick={() => { const n = Number(draft); if (Number.isInteger(n) && n >= 0) void commit(n); }}>Save</Button>
      </div>
    </div>
  );
}

function NumberEditor(props: {
  field: "year" | "hour" | "student_count";
  label: string;
  value: number | null | undefined;
  saving: boolean;
  onSave: (field: string, changes: CourseEditChanges) => Promise<boolean>;
  close: () => void;
}) {
  const { field, label, value, saving, onSave, close } = props;
  const [draft, setDraft] = useState(value == null ? "" : String(value));
  const [submitting, setSubmitting] = useState(false);
  const committingRef = { current: false };
  // Set by Escape so the blur that fires when the focused input is removed
  // from the DOM (browsers fire blur on removal) cannot commit a cancelled
  // edit as a save.
  const discardRef = { current: false };

  const commit = async () => {
    if (committingRef.current || submitting || saving) return;
    if (discardRef.current) {
      discardRef.current = false;
      close();
      return;
    }
    const raw = draft.trim();
    if (raw === "") return;
    const num = Math.floor(Number(raw));
    if (!Number.isFinite(num) || num < 0) return;
    if (num === value) {
      close();
      return;
    }
    committingRef.current = true;
    setSubmitting(true);
    const ok = await onSave(field, { [field]: num } as CourseEditChanges);
    setSubmitting(false);
    committingRef.current = false;
    if (ok) close();
  };

  return (
    <div className="w-full space-y-3 p-2">
      <p className="px-1 pt-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">
        {label}
      </p>
      <input
        autoFocus
        type="text"
        inputMode="numeric"
        aria-label={label}
        value={draft}
        onChange={(e) => {
          discardRef.current = false;
          setDraft(e.target.value);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            void commit();
          } else if (e.key === "Escape") {
            e.preventDefault();
            discardRef.current = true;
            setDraft(value == null ? "" : String(value));
            close();
          }
        }}
        onBlur={() => {
          if (!committingRef.current) void commit();
        }}
        disabled={submitting || saving}
        className="h-8 w-full rounded-sm border border-wi-line bg-white px-2.5 text-sm text-[var(--color-wi-text)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15 disabled:opacity-60"
      />
      <div className="flex justify-end gap-2 pt-1">
        <Button variant="secondary" size="sm" onMouseDown={(e) => e.preventDefault()} onClick={close}>
          Cancel
        </Button>
        <Button variant="primary" size="sm" onClick={() => void commit()} loading={submitting || saving}>
          Save
        </Button>
      </div>
    </div>
  );
}
