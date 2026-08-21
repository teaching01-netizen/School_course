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

interface CreateSessionPopoverProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The button this creator is anchored to. */
  trigger: ReactElement<{ ref?: Ref<HTMLElement>; onClick?: React.MouseEventHandler }>;
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
  onCreate: () => void;
  /** Progressive disclosure: the heavier creation flows open from here. */
  onOpenSeries: () => void;
  onOpenPaste: () => void;
}

/**
 * Anchored "New session" creator. Creating a one-off session is the common
 * case, so it lives in the same quiet popover grammar as editing — the course
 * is the page context rather than a form field, and the availability strip
 * reports the result before anything is created.
 */
export default function CreateSessionPopover(props: CreateSessionPopoverProps) {
  const { open, onOpenChange, trigger, ...panelProps } = props;

  return (
    <Popover
      open={open}
      onOpenChange={onOpenChange}
      trigger={trigger}
      ariaLabel="New session"
      contentClassName="w-[23rem] max-w-[calc(100vw-1rem)]"
    >
      <CreateSessionPanel {...panelProps} onOpenChange={onOpenChange} />
    </Popover>
  );
}

function CreateSessionPanel(props: Omit<CreateSessionPopoverProps, "open" | "trigger" | "onOpenChange"> & { onOpenChange: (open: boolean) => void }) {
  const {
    form, setForm, preflight, canSave, saving,
    rooms, teacherOptions, course,
    coursesById, teachersById, roomsById, onCreate, onOpenSeries, onOpenPaste, onOpenChange,
  } = props;

  const date = form.start_local.slice(0, 10);
  const begin = form.start_local.length >= 16 ? form.start_local.slice(11, 16) : "";
  const end = form.end_local.length >= 16 ? form.end_local.slice(11, 16) : "";

  const dateRef = useRef<HTMLInputElement | null>(null);
  const startRef = useRef<HTMLInputElement | null>(null);
  const endRef = useRef<HTMLInputElement | null>(null);
  const roomRef = useRef<HTMLSelectElement | null>(null);

  // Land on the first control a new session needs: the date.
  useEffect(() => {
    if (dateRef.current) dateRef.current.focus();
  }, []);

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

  const createOnEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== "Enter") return;
    e.preventDefault();
    if (canSave && !saving) onCreate();
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
        <p className="text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-text-light)]">New session</p>
        <p className="mt-0.5 text-sm font-medium text-[var(--color-wi-text)]">
          {summary || "Pick a date and time"}
        </p>
        <p className="truncate text-xs text-[var(--color-wi-text-light)]">
          {course ? `${course.code} — ${course.name}` : ""} · {classroomLabel} · {teacherLabel}
        </p>
      </div>

      <div className="space-y-0.5">
        <PropertyRow label="Date" htmlFor="session-create-date">
          <Input
            ref={dateRef}
            id="session-create-date"
            type="date"
            size="sm"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            onKeyDown={createOnEnter}
          />
        </PropertyRow>
        <PropertyRow label="Start" htmlFor="session-create-start">
          <Input
            ref={startRef}
            id="session-create-start"
            type="time"
            step={300}
            size="sm"
            value={begin}
            onChange={(e) => setBegin(e.target.value)}
            onKeyDown={createOnEnter}
          />
        </PropertyRow>
        <PropertyRow label="End" htmlFor="session-create-end">
          <Input
            ref={endRef}
            id="session-create-end"
            type="time"
            step={300}
            size="sm"
            value={end}
            onChange={(e) => setEnd(e.target.value)}
            onKeyDown={createOnEnter}
          />
        </PropertyRow>
        <PropertyRow label="Classroom" htmlFor="session-create-room">
          <Select ref={roomRef} id="session-create-room" size="sm" value={form.room_id} onChange={(e) => setForm((prev) => ({ ...prev, room_id: e.target.value }))}>
            <option value="">Not set (provisional)</option>
            {rooms.map((r) => (
              <option key={r.id} value={r.id}>{r.name}</option>
            ))}
          </Select>
        </PropertyRow>
        <PropertyRow label="Teacher" htmlFor="session-create-teacher">
          <TypeaheadSelect
            id="session-create-teacher"
            value={form.teacher_id}
            onChange={(v) => setForm((prev) => ({ ...prev, teacher_id: v }))}
            options={teacherOptions}
            placeholder="Search teacher…"
          />
        </PropertyRow>
      </div>

      <SessionAvailabilityStatus
        preflight={preflight}
        coursesById={coursesById}
        teachersById={teachersById}
        roomsById={roomsById}
        missingFields={missingFields}
        roomMissing={!form.room_id}
        actionVerb="create"
      />

      <div className="space-y-1.5 border-t border-wi-line-soft pt-2">
        <div className="flex items-center gap-1.5 px-0.5">
          <button
            type="button"
            onClick={onOpenSeries}
            className="rounded px-1.5 py-1 text-xs text-[var(--color-wi-text-light)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] motion-reduce:transition-none"
          >
            Recurring series…
          </button>
          <button
            type="button"
            onClick={onOpenPaste}
            className="rounded px-1.5 py-1 text-xs text-[var(--color-wi-text-light)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:shadow-[inset_0_0_0_2px_var(--color-wi-primary)] motion-reduce:transition-none"
          >
            Paste a schedule…
          </button>
        </div>
        <div className="flex items-center justify-end gap-2">
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)} disabled={saving}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={onCreate}
            disabled={!canSave || saving}
            loading={saving}
          >
            {saving ? "Creating…" : getSessionSaveLabel(preflight, "Create session", "Create as provisional")}
          </Button>
        </div>
      </div>
    </div>
  );
}