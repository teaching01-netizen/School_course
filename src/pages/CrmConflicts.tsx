import { Fragment, useCallback, useEffect, useState } from "react";
import { apiJson } from "../api/client";
import { useToast } from "../hooks/useToast";
import { crmCourseLabel, formatCRMConflictTime } from "../utils/crmConflict";
import PageHeading from "../components/ui/PageHeading";
import LoadingSkeleton from "../components/ui/LoadingSkeleton";
import EmptyState from "../components/ui/EmptyState";
import {
  CRMConflictResolutionPanel,
  type ResolveConflictResponse,
} from "../components/crm/CRMConflictResolutionPanel";
import { ChevronRight, Clock, AlertTriangle } from "lucide-react";

type ConflictCourse = {
  id: string;
  code: string;
  name: string;
  subject_name: string;
};

type ConflictStudent = {
  id: string;
  wcode: string;
  full_name: string;
};

type ConflictSession = {
  session_id: string;
  course?: ConflictCourse;
  start_at: string;
  end_at: string;
};

type TargetSession = {
  session_id: string;
  start_at: string;
  end_at: string;
  label?: string;
};

type ConflictItem = {
  job_id: string;
  course: ConflictCourse;
  student: ConflictStudent;
  conflicts: ConflictSession[];
  target_sessions: TargetSession[];
  created_at: string;
};

function formatDetectedAt(iso: string): string {
  try {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleDateString("en-GB", {
      day: "numeric",
      month: "short",
      year: "numeric",
      timeZone: "Asia/Bangkok",
    });
  } catch {
    return iso;
  }
}

function conflictSummary(conflicts: ConflictSession[]): string {
  if (conflicts.length === 0) return "—";
  const first = conflicts[0];
  const label = crmCourseLabel(first.course) || "unknown course";
  return `${conflicts.length} overlapping with ${label}`;
}

export default function CrmConflicts() {
  const { addToast } = useToast();
  const [conflicts, setConflicts] = useState<ConflictItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expandedIds, setExpandedIds] = useState<Set<string>>(() => new Set());

  const fetchConflicts = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await apiJson<ConflictItem[]>("/api/v1/crm/conflicts", { method: "GET" });
      setConflicts(data ?? []);
    } catch (err: any) {
      const msg = err?.message ?? "Failed to load CRM conflicts";
      setError(msg);
      addToast("error", msg);
    } finally {
      setLoading(false);
    }
  }, [addToast]);

  useEffect(() => {
    void fetchConflicts();
  }, [fetchConflicts]);

  const handleToggleExpand = (jobId: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(jobId)) {
        next.delete(jobId);
      } else {
        next.add(jobId);
      }
      return next;
    });
  };

  const handleConflictResolved = (jobId: string, _response: ResolveConflictResponse) => {
    setConflicts((prev) => prev.filter((c) => c.job_id !== jobId));
    setExpandedIds((prev) => {
      const next = new Set(prev);
      next.delete(jobId);
      return next;
    });
  };

  const toBusyRangeConflict = (item: ConflictItem) => {
    const firstConflict = item.conflicts[0];
    return {
      studentWCode: item.student.wcode,
      studentName: item.student.full_name,
      targetCourse: crmCourseLabel(item.course),
      targetCourseID: item.course.id,
      conflictingCourse: crmCourseLabel(firstConflict?.course),
      conflictTime: formatCRMConflictTime(firstConflict?.start_at, firstConflict?.end_at),
      targetSessions: item.target_sessions,
      detail: `${item.student.full_name} (${item.student.wcode}) conflicts with ${item.conflicts.length} session(s)`,
    };
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <PageHeading>Schedule Conflicts</PageHeading>
          <p className="text-sm text-[var(--color-wi-text-light)] mt-1">
            Unresolved CRM roster conflicts that require manual resolution
          </p>
        </div>
        <button
          onClick={() => void fetchConflicts()}
          disabled={loading}
          className="px-3 py-1.5 text-sm rounded-md border border-[var(--color-wi-line)] text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] transition-colors disabled:opacity-50"
        >
          Refresh
        </button>
      </div>

      {loading && <LoadingSkeleton type="table" lines={4} />}

      {error && !loading && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 flex items-center gap-2">
          <AlertTriangle className="w-4 h-4 shrink-0" />
          {error}
        </div>
      )}

      {!loading && !error && conflicts.length === 0 && (
        <EmptyState message="No pending schedule conflicts" />
      )}

      {!loading && conflicts.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-[13px]">
            <caption className="sr-only">Schedule conflicts</caption>
            <thead>
              <tr className="border-b border-b-[var(--color-wi-line)]">
                <th scope="col" className="w-8 px-2"></th>
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Student</th>
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Course</th>
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Conflict</th>
                <th scope="col" className="text-left py-2 px-2 font-semibold text-[var(--color-wi-text-light)]">Detected</th>
              </tr>
            </thead>
            <tbody>
              {conflicts.map((item) => (
                <Fragment key={item.job_id}>
                  <tr className="border-b border-b-[var(--color-wi-line)] hover:bg-[var(--color-wi-row-alt)]">
                    <td className="w-8 py-3 px-1">
                      <button
                        type="button"
                        onClick={() => handleToggleExpand(item.job_id)}
                        className="flex items-center justify-center h-6 w-6 rounded-sm text-[var(--color-wi-text-light)] hover:text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] cursor-pointer"
                        aria-label={expandedIds.has(item.job_id) ? "Collapse resolution" : "Expand resolution"}
                        aria-expanded={expandedIds.has(item.job_id)}
                      >
                        <ChevronRight
                          className={`h-4 w-4 transition-transform duration-150 ${
                            expandedIds.has(item.job_id) ? "rotate-90" : ""
                          }`}
                        />
                      </button>
                    </td>
                    <td className="py-3 px-2">
                      <span className="font-medium text-[var(--color-wi-text)]">{item.student.full_name}</span>
                      <span className="text-[var(--color-wi-text-light)] ml-1">({item.student.wcode})</span>
                    </td>
                    <td className="py-3 px-2 text-[var(--color-wi-text-light)]">
                      {crmCourseLabel(item.course) || item.course.code || "—"}
                    </td>
                    <td className="py-3 px-2 text-[var(--color-wi-text-light)]">
                      <span className="inline-flex items-center gap-1">
                        <Clock className="w-3.5 h-3.5 text-amber-500" />
                        {conflictSummary(item.conflicts)}
                      </span>
                    </td>
                    <td className="py-3 px-2 text-[var(--color-wi-text-light)] text-xs whitespace-nowrap">
                      {formatDetectedAt(item.created_at)}
                    </td>
                  </tr>
                  {expandedIds.has(item.job_id) && (
                    <tr className="border-b border-b-[var(--color-wi-line)]">
                      <td colSpan={5} className="p-0">
                        <div className="px-8 py-3 bg-[var(--color-wi-row-alt)]/50">
                          <CRMConflictResolutionPanel
                            conflict={toBusyRangeConflict(item)}
                            onResolved={(response) => handleConflictResolved(item.job_id, response)}
                          />
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
