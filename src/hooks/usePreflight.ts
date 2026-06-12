import { useState, useCallback, useEffect, useRef } from "react";
import { ApiRequestError, apiJson } from "@/api/client";
import type { ConflictDetails } from "@/types";
import { isConflictDetails } from "@/utils/conflictErrors";

export type PreflightStatus = "available" | "provisional" | "blocked" | "idle";

export type PreflightParams = {
  course_id: string;
  teacher_id: string;
  room_id: string | null;
  start_at: string;
  end_at: string;
  session_id?: string | null;
  series_id?: string | null;
  included_student_ids?: string[];
  excluded_student_ids?: string[];
  weekdays?: number[];
  start_local_time?: string;
  duration_minutes?: number;
  start_date?: string;
  end_date?: string | null;
  count?: number | null;
};

export type UsePreflightReturn = {
  status: PreflightStatus;
  loading: boolean;
  details: ConflictDetails | null;
  error: ApiRequestError | null;
  occurrencesPlanned: number | null;
  check: (params: PreflightParams) => Promise<void>;
  reset: () => void;
};

export function usePreflight(endpoint: "preflight" | "preflight_series" = "preflight"): UsePreflightReturn {
  const [status, setStatus] = useState<PreflightStatus>("idle");
  const [loading, setLoading] = useState(false);
  const [details, setDetails] = useState<ConflictDetails | null>(null);
  const [error, setError] = useState<ApiRequestError | null>(null);
  const [occurrencesPlanned, setOccurrencesPlanned] = useState<number | null>(null);
  const mountedRef = useRef(false);
  const checkIdRef = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; };
  }, []);

  const safe = {
    setStatus: (s: PreflightStatus) => { if (mountedRef.current) setStatus(s); },
    setLoading: (v: boolean) => { if (mountedRef.current) setLoading(v); },
    setDetails: (d: ConflictDetails | null) => { if (mountedRef.current) setDetails(d); },
    setError: (e: ApiRequestError | null) => { if (mountedRef.current) setError(e); },
    setOccurrencesPlanned: (v: number | null) => { if (mountedRef.current) setOccurrencesPlanned(v); },
  };

  const check = useCallback(async (params: PreflightParams) => {
    const thisCheckId = ++checkIdRef.current;
    console.debug("[usePreflight] check:start", { endpoint, thisCheckId, params });
    safe.setLoading(true);
    safe.setStatus("idle");
    safe.setDetails(null);
    safe.setError(null);
    safe.setOccurrencesPlanned(null);

    try {
      let url: string;
      let body: Record<string, unknown>;

      if (endpoint === "preflight_series") {
        url = "/api/v1/scheduling/preflight_series";
        body = {
          course_id: params.course_id,
          room_id: params.room_id,
          teacher_id: params.teacher_id,
          weekdays: params.weekdays ?? [],
          start_local_time: params.start_local_time ?? "",
          duration_minutes: params.duration_minutes ?? 0,
          start_date: params.start_date ?? "",
          end_date: params.end_date ?? null,
          count: params.count ?? null,
        };
      } else {
        url = "/api/v1/scheduling/preflight";
        body = {
          session_id: params.session_id ?? null,
          course_id: params.course_id,
          room_id: params.room_id,
          teacher_id: params.teacher_id,
          start_at: params.start_at,
          end_at: params.end_at,
        };
        if (params.included_student_ids?.length || params.excluded_student_ids?.length) {
          body.included_student_ids = params.included_student_ids;
          body.excluded_student_ids = params.excluded_student_ids;
        }
      }

      const res = await apiJson<{ status: "available" | "provisional"; occurrences_planned?: number }>(url, {
        method: "POST",
        body: JSON.stringify(body),
      });

      if (thisCheckId !== checkIdRef.current) return;
      console.debug("[usePreflight] check:success", { endpoint, thisCheckId, status: res.status, occurrences_planned: res.occurrences_planned ?? null });
      safe.setStatus(res.status);
      safe.setOccurrencesPlanned(res.occurrences_planned ?? null);
    } catch (err) {
      if (thisCheckId !== checkIdRef.current) return;
      console.debug("[usePreflight] check:error", {
        endpoint,
        thisCheckId,
        isApiRequestError: err instanceof ApiRequestError,
        status: err instanceof ApiRequestError ? err.status : undefined,
        code: err instanceof ApiRequestError ? err.code : undefined,
        details: err instanceof ApiRequestError ? err.details : undefined,
      });
      safe.setStatus("blocked");
      if (err instanceof ApiRequestError) {
        safe.setError(err);
        safe.setDetails(isConflictDetails(err.details) ? err.details : null);
      } else {
        safe.setError(new ApiRequestError("Preflight failed"));
        safe.setDetails(null);
      }
    } finally {
      if (thisCheckId !== checkIdRef.current) return;
      console.debug("[usePreflight] check:done", { endpoint, thisCheckId });
      safe.setLoading(false);
    }
  }, [endpoint]);

  const reset = useCallback(() => {
    safe.setStatus("idle");
    safe.setLoading(false);
    safe.setDetails(null);
    safe.setError(null);
    safe.setOccurrencesPlanned(null);
  }, []);

  return { status, loading, details, error, occurrencesPlanned, check, reset };
}
