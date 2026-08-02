import { useState, useCallback, useEffect, useRef } from "react";
import { ApiRequestError, apiJson } from "@/api/client";
import { isConflictDetails } from "@/utils/conflictErrors";
import type { ConflictDetails } from "../types";

export type PreflightStatus =
  | "idle"
  | "checking"
  | "available"
  | "provisional"
  | "blocked"
  | "error";

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
  lastParams: PreflightParams | null;
  check: (params: PreflightParams) => Promise<void>;
  reset: () => void;
};

export function isSchedulingConflict(error: unknown): error is ApiRequestError {
  return error instanceof ApiRequestError && error.status === 409 && isConflictDetails(error.details);
}

export function usePreflight(endpoint: "preflight" | "preflight_series" = "preflight"): UsePreflightReturn {
  const [status, setStatus] = useState<PreflightStatus>("idle");
  const [loading, setLoading] = useState(false);
  const [details, setDetails] = useState<ConflictDetails | null>(null);
  const [error, setError] = useState<ApiRequestError | null>(null);
  const [occurrencesPlanned, setOccurrencesPlanned] = useState<number | null>(null);
  const mountedRef = useRef(false);
  const controllerRef = useRef<AbortController | null>(null);
  const lastParamsRef = useRef<PreflightParams | null>(null);
  const [lastParams, setLastParams] = useState<PreflightParams | null>(null);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      controllerRef.current?.abort();
    };
  }, []);

  const safe = {
    setStatus: (s: PreflightStatus) => { if (mountedRef.current) setStatus(s); },
    setLoading: (v: boolean) => { if (mountedRef.current) setLoading(v); },
    setDetails: (d: ConflictDetails | null) => { if (mountedRef.current) setDetails(d); },
    setError: (e: ApiRequestError | null) => { if (mountedRef.current) setError(e); },
    setOccurrencesPlanned: (v: number | null) => { if (mountedRef.current) setOccurrencesPlanned(v); },
    setLastParams: (p: PreflightParams | null) => { if (mountedRef.current) { setLastParams(p); lastParamsRef.current = p; } },
  };

  const check = useCallback(async (params: PreflightParams) => {
    // Abort any previous in-flight request
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;

    safe.setStatus("checking");
    safe.setLoading(true);
    safe.setDetails(null);
    safe.setError(null);
    safe.setLastParams(params);

    try {
      let url: string;
      let body: Record<string, unknown>;

      if (endpoint === "preflight_series") {
        url = "/api/v1/scheduling/preflight_series";
        body = {
          series_id: params.series_id ?? null,
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
        signal: controller.signal,
        body: JSON.stringify(body),
      });

      if (controller.signal.aborted) return;
      safe.setStatus(res.status);
      safe.setOccurrencesPlanned(res.occurrences_planned ?? null);
    } catch (err) {
      if (controller.signal.aborted) return;

      // Classify: real scheduling conflict vs system error
      if (isSchedulingConflict(err)) {
        safe.setStatus("blocked");
        safe.setDetails(err.details as ConflictDetails);
        safe.setError(err);
      } else if (err instanceof ApiRequestError) {
        safe.setStatus("error");
        safe.setError(err);
        safe.setDetails(isConflictDetails(err.details) ? err.details : null);
      } else {
        safe.setStatus("error");
        safe.setError(new ApiRequestError(
          err instanceof Error ? err.message : "Unknown error",
        ));
        safe.setDetails(null);
      }
    } finally {
      if (!controller.signal.aborted && mountedRef.current) {
        safe.setLoading(false);
      }
    }
  }, [endpoint]);

  const reset = useCallback(() => {
    controllerRef.current?.abort();
    safe.setStatus("idle");
    safe.setLoading(false);
    safe.setDetails(null);
    safe.setError(null);
    safe.setOccurrencesPlanned(null);
    safe.setLastParams(null);
  }, []);

  return { status, loading, details, error, occurrencesPlanned, lastParams, check, reset };
}
