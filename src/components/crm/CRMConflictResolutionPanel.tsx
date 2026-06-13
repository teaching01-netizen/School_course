import { useEffect, useState } from "react";
import { apiJson } from "../../api/client";
import { useToast } from "../../hooks/useToast";
import {
  formatCRMConflictTime,
  type CRMStudentScheduleConflictDetails,
} from "../../utils/crmConflict";
import { AlertTriangle, CheckCircle2, Loader2 } from "lucide-react";

export type BusyRangeConflict = {
  studentWCode: string | null;
  studentName?: string | null;
  targetCourse?: string | null;
  targetCourseID?: string | null;
  conflictingCourse?: string | null;
  conflictTime?: string | null;
  targetSessions?: NonNullable<CRMStudentScheduleConflictDetails["target_sessions"]>;
  detail: string;
};

export type ResolveConflictResponse = {
  ok: boolean;
  job_id?: string;
  status: string;
};

export function CRMConflictResolutionPanel({
  conflict,
  onResolved,
}: {
  conflict: BusyRangeConflict;
  onResolved: (response: ResolveConflictResponse) => void;
}) {
  const { addToast } = useToast();
  const targetSessions = conflict.targetSessions ?? [];
  const [selectedIDs, setSelectedIDs] = useState<string[]>(() =>
    targetSessions.map((session) => session.session_id).filter((id): id is string => !!id),
  );
  const [submitting, setSubmitting] = useState(false);
  const [submitAttempted, setSubmitAttempted] = useState(false);

  useEffect(() => {
    setSelectedIDs(targetSessions.map((session) => session.session_id).filter((id): id is string => !!id));
  }, [targetSessions]);

  if (!conflict.studentWCode || !conflict.targetCourseID || targetSessions.length === 0) {
    return null;
  }

  const studentWCode = conflict.studentWCode;
  const targetCourseID = conflict.targetCourseID;
  const selectedSet = new Set(selectedIDs);
  const selectedError = submitAttempted && selectedIDs.length === 0;

  const toggleSession = (sessionID: string, checked: boolean) => {
    setSelectedIDs((current) => {
      if (checked) return current.includes(sessionID) ? current : [...current, sessionID];
      return current.filter((id) => id !== sessionID);
    });
  };

  const submitResolution = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitAttempted(true);
    if (selectedIDs.length === 0) return;

    try {
      setSubmitting(true);
      const response = await apiJson<ResolveConflictResponse>(
        `/api/v1/crm/students/${encodeURIComponent(studentWCode)}/resolve-conflict`,
        {
          method: "POST",
          body: JSON.stringify({
            course_id: targetCourseID,
            excluded_session_ids: selectedIDs,
          }),
        },
      );
      addToast("success", "Conflict exclusions saved — reconcile queued");
      onResolved(response);
    } catch (err: any) {
      addToast("error", err?.message ?? "Failed to resolve CRM conflict");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submitResolution} className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-3">
      <div className="flex items-start gap-2">
        <AlertTriangle className="w-4 h-4 text-amber-600 mt-0.5 shrink-0" />
        <div className="min-w-0 flex-1">
          <p className="text-xs font-semibold text-amber-900">Resolve as cross-enrollment</p>
          <p className="mt-1 text-xs text-amber-800">
            Exclude selected sessions from {conflict.targetCourse ?? "this course"} for this student, then rerun reconcile.
          </p>
        </div>
      </div>

      <fieldset className="mt-3 space-y-2">
        <legend className="text-xs font-medium text-amber-900">Target course sessions to exclude</legend>
        {targetSessions.map((session, index) => {
          const sessionID = session.session_id;
          if (!sessionID) return null;
          const label = session.label || `Session ${index + 1}`;
          const time = formatCRMConflictTime(session.start_at, session.end_at);
          return (
            <label
              key={sessionID}
              className="flex items-start gap-2 rounded-md border border-amber-200 bg-white px-2.5 py-2 text-xs text-gray-700"
            >
              <input
                name="excluded_session_ids"
                type="checkbox"
                className="mt-0.5 h-4 w-4 accent-emerald-600"
                checked={selectedSet.has(sessionID)}
                onChange={(event) => toggleSession(sessionID, event.currentTarget.checked)}
                disabled={submitting}
              />
              <span>
                <span className="font-medium text-gray-900">{label}</span>
                {time ? <span className="text-gray-500"> — {time}</span> : null}
              </span>
            </label>
          );
        })}
      </fieldset>
      {selectedError && (
        <p className="mt-2 text-xs text-red-700" role="alert">
          Select at least one session to exclude.
        </p>
      )}

      <div className="mt-3 flex justify-end">
        <button
          type="submit"
          disabled={submitting}
          className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-600 px-3 py-2 text-xs font-medium text-white shadow-sm transition-colors hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {submitting ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <CheckCircle2 className="w-3.5 h-3.5" />}
          Confirm & rerun reconcile
        </button>
      </div>
    </form>
  );
}
