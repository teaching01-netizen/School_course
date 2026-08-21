import { useEffect, useRef, type Dispatch, type ReactElement, type Ref, type SetStateAction } from "react";
import { format, parseISO } from "date-fns";
import { Popover } from "@/components/ui/Popover";
import Button from "@/components/ui/Button";
import Input from "@/components/ui/Input";
import Select from "@/components/ui/Select";
import TypeaheadSelect, { type TypeaheadOption } from "@/components/TypeaheadSelect";
import { getSessionSaveLabel, SessionAvailabilityStatus } from "./AvailabilityStatus";
import { PropertyRow } from "./SessionPropertyRow";
import type { EditSessionForm } from "@/features/scheduling/hooks/useEditSession";
import type { UsePreflightReturn } from "@/features/scheduling/hooks/usePreflight";
import type { Course } from "@/features/courses/types";
import type { User } from "@/types/shared";
import type { Room } from "@/features/scheduling/types";

export type SessionEditorFocusField = "date" | "start" | "end" | "room" | "teacher";

interface SessionEditorPopoverProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The cell or card this editor is anchored to. */
  trigger: ReactElement<{ ref?: Ref<HTMLElement>; onClick?: React.MouseEventHandler }>;
  /** Which field receives focus when the editor opens. */
  focusField: SessionEditorFocusField;
  form: EditSessionForm;
  setForm: Dispatch<SetStateAction<EditSessionForm>>;
  preflight: UsePreflightReturn;
  canSave: boolean;
  saving: boolean;
  rooms: Room[];
  teacherOptions: TypeaheadOption[];
  course: Course | null;
  coursesById: Map<string, Course>;
  teachersById: Map<string, User>;
  roomsById?: Map<string, Room>;
  onSave: () => void;
}

/**
 * Anchored session editor: the Notion property-row grammar applied to a
 * schedule entry. Editing happens in a focused popover so the schedule table
 * never shifts layout, and the availability strip keeps the preflight result
 * readable at a glance.
 */
export default function SessionEditorPopover(props: SessionEditorPopoverProps) {
  const { open, onOpenChange, trigger, ...panelProps } = props;

  return (
    <Popover
      open={open}
      onOpenChange={onOpenChange}
      trigger={trigger}
      autoFocus={false}
      ariaLabel="Edit session"
      contentClassName="w-[23rem] max-w-[calc(100vw-1rem)]"
    >
      <SessionEditorPanel {...panelProps} onOpenChange={onOpenChange} />
    </Popover>
  );
}

function SessionEditorPanel(props: Omit<SessionEditorPopoverProps, "open" | "trigger" | "onOpenChange"> & { onOpenChange: (open: boolean) => void }) {
  const {
    focusField, form, setForm, preflight, canSave, saving,
    rooms, teacherOptions, course,
    coursesById, teachersById, roomsById, onSave, onOpenChange,
  } = props;

  const date = form.start_local.slice(0, 10);
  const begin = form.start_local.length >= 16 ? form.start_local.slice(11, 16) : "";
  const end = form.end_local.length >= 16 ? form.end_local.slice(11, 16) : "";

  const dateRef = useRef<HTMLInputElement | null>(null);
  const startRef = useRef<HTMLInputElement | null>(null);
  const endRef = useRef<HTMLInputElement | null>(null);
  const roomRef = useRef<HTMLSelectElement | null>(null);
  const teacherRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    switch (focusField) {
      case "date": dateRef.current?.focus(); break;
      case "start": startRef.current?.focus(); break;
      case "end": endRef.current?.focus(); break;
      case "room": roomRef.current?.focus(); break;
      case "teacher": teacherRef.current?.querySelector<HTMLInputElement>("input")?.focus(); break;
    }
  }, [focusField]);

  const setDate = (v: string) => {
    setForm((prev) => ({
      ...prev,
      start_local: v ? `${v}T${prev.start_local.slice(11, 16) || "00:00"}` : "",
      end_local: v ? `${v}T${prev.end_local.slice(11, 16) || "00:00"}` : "",
    }));
  };
  const setBegin = (v: string) => {
    setForm((prev) => {
      const d = prev.start_local.slice(0, 10);
      return d ? { ...prev, start_local: `${d}T${v}` } : prev;
    });
  };
  const setEnd = (v: string) => {
    setForm((prev) => {
      const d = prev.start_local.slice(0, 10);
      return d ? { ...prev, end_local: `${d}T${v}` } : prev;
    });
  };

  const saveOnEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== "Enter") return;
    e.preventDefault();
    if (canSave && !saving) onSave();
  };

  const summary = [
    date ? format(parseISO(date), "EEE d MMM yyyy") : null,
    begin && end ? `${begin}–${end}` : null,
  ].filter(Boolean).join(", ");

  const classroomLabel = rooms.find((r) => r.id === form.room_id)?.name ?? "No classroom";
  const teacherLabel = teacherOptions.find((o) => o.value === form.teacher_id)?.label ?? "No teacher";
  const missingFields = [!date ? "Date" : "", !begin || !end ? "time" : "", !form.teacher_id ? "Teacher" : ""].filter(Boolean);

  return (
    <div className="space-y-2 p-2">
      <div className="px-1.5 pt-0.5">
        <p className="text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">Edit session</p>
        <p className="mt-0.5 text-sm font-medium text-[var(--color-wi-text)]">
          {summary || "Set a date and time"}
        </p>
        <p className="truncate text-xs text-[var(--color-wi-text-light)]">
          {course ? `${course.code} — ${course.name}` : ""} · {classroomLabel} · {teacherLabel}
        </p>
      </div>

      <div className="space-y-0.5">
        <PropertyRow label="Date" htmlFor="session-editor-date">
          <Input
            ref={dateRef}
            id="session-editor-date"
            type="date"
            size="sm"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            onKeyDown={saveOnEnter}
          />
        </PropertyRow>
        <PropertyRow label="Start" htmlFor="session-editor-start">
          <Input
            ref={startRef}
            id="session-editor-start"
            type="time"
            step={300}
            size="sm"
            value={begin}
            onChange={(e) => setBegin(e.target.value)}
            onKeyDown={saveOnEnter}
          />
        </PropertyRow>
        <PropertyRow label="End" htmlFor="session-editor-end">
          <Input
            ref={endRef}
            id="session-editor-end"
            type="time"
            step={300}
            size="sm"
            value={end}
            onChange={(e) => setEnd(e.target.value)}
            onKeyDown={saveOnEnter}
          />
        </PropertyRow>
        <PropertyRow label="Classroom" htmlFor="session-editor-room">
          <Select ref={roomRef} id="session-editor-room" size="sm" value={form.room_id} onChange={(e) => setForm((prev) => ({ ...prev, room_id: e.target.value }))}>
            <option value="">Not set (provisional)</option>
            {rooms.map((r) => (
              <option key={r.id} value={r.id}>{r.name}</option>
            ))}
          </Select>
        </PropertyRow>
        <PropertyRow label="Teacher" htmlFor="session-editor-teacher">
          <div ref={teacherRef}>
            <TypeaheadSelect
              id="session-editor-teacher"
              value={form.teacher_id}
              onChange={(v) => setForm((prev) => ({ ...prev, teacher_id: v }))}
              options={teacherOptions}
              placeholder="Search teacher…"
            />
          </div>
        </PropertyRow>
      </div>

      <SessionAvailabilityStatus
        preflight={preflight}
        coursesById={coursesById}
        teachersById={teachersById}
        roomsById={roomsById}
        missingFields={missingFields}
        roomMissing={!form.room_id}
      />

      <div className="flex items-center justify-end gap-2 border-t border-wi-line-soft pt-2">
        <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)} disabled={saving}>
          Cancel
        </Button>
        <Button
          variant="primary"
          size="sm"
          onClick={onSave}
          disabled={!canSave || saving}
          loading={saving}
        >
          {saving ? "Saving…" : getSessionSaveLabel(preflight)}
        </Button>
      </div>
    </div>
  );
}