import { memo, type Dispatch, type SetStateAction } from "react";
import { Fragment } from "react";
import { Link } from "react-router-dom";
import Button from "../ui/Button";
import SessionActions from "../SessionActions";
import SessionOccurrenceForm, { type SessionOccurrenceFormData } from "../SessionOccurrenceForm";
import { PreflightIndicator, getSaveButtonLabel } from "../PreflightIndicator";
import { formatUTCToZone } from "@/utils/timezone";
import type { UsePreflightReturn } from "@/features/scheduling/hooks/usePreflight";
import type { Course, Room, Session, User } from "@/types";
import type { TypeaheadOption } from "../TypeaheadSelect";

/**
 * Inline-edit state for one session, assembled by the Schedule page. `null`
 * while that session is not being edited, which keeps the prop referentially
 * stable so memoized cards for every other session skip re-rendering while
 * someone types in this one.
 */
export type SessionInlineEdit = {
  form: SessionOccurrenceFormData;
  setForm: Dispatch<SetStateAction<SessionOccurrenceFormData>>;
  preflight: UsePreflightReturn;
  canSave: boolean;
  isChecking: boolean;
  saving: boolean;
  submit: () => void;
  close: () => void;
};

type SessionCardProps = {
  session: Session;
  courseById: Map<string, Course>;
  roomById: Map<string, Room>;
  teacherById: Map<string, User>;
  courseOptions: TypeaheadOption[];
  teacherOptions: TypeaheadOption[];
  rooms: Room[];
  zone: string;
  impacted: boolean;
  cancelingId: string | null;
  inlineEdit: SessionInlineEdit | null;
  onAttendance: (session: Session) => void;
  onEdit: (session: Session) => void;
  onCancel: (session: Session) => void;
  onEditSeriesTandF: (session: Session) => void;
  onEditSeriesEntire: (session: Session) => void;
  onCancelSeries: (session: Session) => void;
  onOpenInlineEdit: (session: Session) => void;
};

function InlineEditForm({
  inlineEdit,
  courseById,
  roomById,
  teacherById,
  courseOptions,
  teacherOptions,
  rooms,
  prefix,
  ariaLabel,
  className,
}: {
  inlineEdit: SessionInlineEdit;
  courseById: Map<string, Course>;
  roomById: Map<string, Room>;
  teacherById: Map<string, User>;
  courseOptions: TypeaheadOption[];
  teacherOptions: TypeaheadOption[];
  rooms: Room[];
  prefix: string;
  ariaLabel: string;
  className: string;
}) {
  return (
    <form aria-label={ariaLabel} className={className} onSubmit={(event) => { event.preventDefault(); inlineEdit.submit(); }}>
      <SessionOccurrenceForm
        form={inlineEdit.form}
        setForm={inlineEdit.setForm}
        courseOptions={courseOptions}
        teacherOptions={teacherOptions}
        rooms={rooms}
        prefix={prefix}
      />
      <PreflightIndicator
        preflight={inlineEdit.preflight}
        coursesById={courseById}
        teachersById={teacherById}
        roomsById={roomById}
        requiredFields={[
          { label: "Course", value: inlineEdit.form.course_id },
          { label: "Teacher", value: inlineEdit.form.teacher_id },
          { label: "Start", value: inlineEdit.form.start_local },
          { label: "End", value: inlineEdit.form.end_local },
        ]}
      />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="secondary" size="sm" onClick={inlineEdit.close}>Cancel</Button>
        <Button
          type="submit"
          variant="primary"
          size="sm"
          disabled={inlineEdit.saving || !inlineEdit.canSave}
          loading={inlineEdit.preflight.loading || inlineEdit.saving}
        >
          {inlineEdit.saving ? "Saving…" : getSaveButtonLabel({ status: inlineEdit.preflight.status, loading: inlineEdit.preflight.loading }, "Save", inlineEdit.preflight.details)}
        </Button>
      </div>
    </form>
  );
}

export const SessionWeekCard = memo(function SessionWeekCard({
  session,
  courseById,
  roomById,
  teacherById,
  courseOptions,
  teacherOptions,
  rooms,
  zone,
  impacted,
  cancelingId,
  inlineEdit,
  onAttendance,
  onEdit,
  onCancel,
  onEditSeriesTandF,
  onEditSeriesEntire,
  onCancelSeries,
  onOpenInlineEdit,
}: SessionCardProps) {
  const course = courseById.get(session.course_id);
  const room = session.room_id ? roomById.get(session.room_id) : null;
  const teacher = teacherById.get(session.teacher_id);
  const inlineLabel = course ? `${course.code} — ${course.name}` : session.course_id;
  const startLabel = formatUTCToZone(session.start_at, zone, "HH:mm") ?? session.start_at.slice(11, 16);
  const endLabel = formatUTCToZone(session.end_at, zone, "HH:mm") ?? session.end_at.slice(11, 16);
  return (
    <div className="w-full text-left border border-wi-line rounded-sm px-2 py-2 hover:bg-[var(--color-wi-row-alt)]">
      <div className="text-xs font-mono text-[var(--color-wi-text-light)]">{startLabel}–{endLabel}</div>
      <div className="text-sm text-[var(--color-wi-text)] font-semibold">{inlineLabel}</div>
      <div className="text-xs text-[var(--color-wi-text-light)]">{(room ? room.name : session.room_id ? session.room_id : "[NOT SET]")} • {teacher ? (teacher.full_name || teacher.username) : session.teacher_id}</div>
      {impacted ? <Link to="/operations/schedule-impact" className="mt-1 inline-block text-xs font-medium text-amber-700 hover:underline">Impact review open</Link> : null}
      <SessionActions
        session={session} cancelingId={cancelingId}
        onAttendance={onAttendance}
        onEdit={onEdit}
        onCancel={onCancel}
        onEditSeriesTandF={onEditSeriesTandF}
        onEditSeriesEntire={onEditSeriesEntire}
        onCancelSeries={onCancelSeries}
      />
      <Button
        variant="ghost"
        size="sm"
        className="mt-1"
        aria-label={`Inline edit session ${inlineLabel}`}
        onClick={() => onOpenInlineEdit(session)}
      >
        Inline Edit
      </Button>
      {inlineEdit && (
        <InlineEditForm
          inlineEdit={inlineEdit}
          courseById={courseById}
          roomById={roomById}
          teacherById={teacherById}
          courseOptions={courseOptions}
          teacherOptions={teacherOptions}
          rooms={rooms}
          prefix={`inline-${session.id}-`}
          ariaLabel={`Inline edit session ${inlineLabel}`}
          className="mt-3 space-y-3"
        />
      )}
    </div>
  );
});

export const SessionTableRow = memo(function SessionTableRow({
  session,
  courseById,
  roomById,
  teacherById,
  courseOptions,
  teacherOptions,
  rooms,
  zone,
  impacted,
  cancelingId,
  inlineEdit,
  onAttendance,
  onEdit,
  onCancel,
  onEditSeriesTandF,
  onEditSeriesEntire,
  onCancelSeries,
  onOpenInlineEdit,
}: SessionCardProps) {
  const label = courseById.get(session.course_id)?.code ?? session.course_id;
  const teacher = teacherById.get(session.teacher_id);
  return (
    <Fragment>
      <tr className="border-b border-wi-line hover:bg-[var(--color-wi-row-alt)]">
        <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{formatUTCToZone(session.start_at, zone, "EEE d MMM yy HH:mm") ?? session.start_at}</td>
        <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{formatUTCToZone(session.end_at, zone, "EEE d MMM yy HH:mm") ?? session.end_at}</td>
        <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{label}</td>
        <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{session.room_id ? (roomById.get(session.room_id)?.name ?? session.room_id) : "[NOT SET]"}</td>
        <td className="py-2 px-2 font-mono text-xs text-[var(--color-wi-text-light)]">{teacher?.full_name || teacher?.username || session.teacher_id}</td>
        <td className="py-2 px-2 text-right">
          {impacted ? <Link to="/operations/schedule-impact" className="mr-2 text-xs font-medium text-amber-700 hover:underline">Impact open</Link> : null}
          <SessionActions
            session={session} cancelingId={cancelingId}
            onAttendance={onAttendance}
            onEdit={onEdit}
            onCancel={onCancel}
            onEditSeriesTandF={onEditSeriesTandF}
            onEditSeriesEntire={onEditSeriesEntire}
            onCancelSeries={onCancelSeries}
          />
          <Button variant="ghost" size="sm" aria-label={`Inline edit session ${label}`} onClick={() => onOpenInlineEdit(session)}>Inline Edit</Button>
        </td>
      </tr>
      {inlineEdit && (
        <tr className="border-b border-wi-line bg-[var(--color-wi-row-alt)]">
          <td colSpan={6} className="px-3 py-3">
            <InlineEditForm
              inlineEdit={inlineEdit}
              courseById={courseById}
              roomById={roomById}
              teacherById={teacherById}
              courseOptions={courseOptions}
              teacherOptions={teacherOptions}
              rooms={rooms}
              prefix={`inline-${session.id}-table-`}
              ariaLabel={`Inline edit session ${label}`}
              className="space-y-3"
            />
          </td>
        </tr>
      )}
    </Fragment>
  );
});
