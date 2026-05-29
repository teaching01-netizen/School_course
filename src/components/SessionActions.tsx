import Button from "./ui/Button";
import type { Session } from "@/types";

interface SessionActionsProps {
  session: Session;
  cancelingId: string | null;
  onAttendance: (s: Session) => void;
  onEdit: (s: Session) => void;
  onCancel: (s: Session) => void;
  onEditSeriesTandF: (s: Session) => void;
  onEditSeriesEntire: (s: Session) => void;
  onCancelSeries: (s: Session) => void;
}

export default function SessionActions({
  session, cancelingId, onAttendance, onEdit, onCancel,
  onEditSeriesTandF, onEditSeriesEntire, onCancelSeries,
}: SessionActionsProps) {
  const isSeries = !!session.series_id;
  return (
    <div className="flex flex-wrap gap-1 mt-1">
      <Button variant="ghost" size="sm" onClick={() => onAttendance(session)}>Attendance</Button>
      {isSeries ? (
        <>
          <Button variant="ghost" size="sm" onClick={() => onEdit(session)}>Edit</Button>
          <Button variant="danger" size="sm" onClick={() => onCancel(session)} disabled={cancelingId === session.id}>
            {cancelingId === session.id ? "Canceling…" : "Cancel"}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => onEditSeriesTandF(session)}>This & Future</Button>
          <Button variant="ghost" size="sm" onClick={() => onEditSeriesEntire(session)}>Edit Series</Button>
          <Button variant="danger" size="sm" onClick={() => onCancelSeries(session)} disabled={cancelingId === session.id}>
            {cancelingId === session.id ? "Canceling…" : "Cancel Series"}
          </Button>
        </>
      ) : (
        <>
          <Button variant="ghost" size="sm" onClick={() => onEdit(session)}>Edit</Button>
          <Button variant="danger" size="sm" onClick={() => onCancel(session)} disabled={cancelingId === session.id}>
            {cancelingId === session.id ? "Canceling…" : "Cancel"}
          </Button>
        </>
      )}
    </div>
  );
}
