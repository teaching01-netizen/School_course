import { useState, useEffect, useCallback, useRef } from "react";
import { ApiRequestError, apiJson } from "@/api/client";

export type UseApiQueryResult<T> = {
  data: T | null;
  loading: boolean;
  error: ApiRequestError | null;
  refetch: () => Promise<void>;
};

export function useApiQuery<T>(url: string | null, deps?: unknown[]): UseApiQueryResult<T> {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(url != null);
  const [error, setError] = useState<ApiRequestError | null>(null);
  const [fetchId, setFetchId] = useState(0);
  const mountedRef = useRef(true);
  const serialized = url ? JSON.stringify([url, ...(deps ?? []), fetchId]) : null;

  useEffect(() => {
    mountedRef.current = true;
    return () => { mountedRef.current = false; };
  }, []);

  useEffect(() => {
    if (url == null) {
      setData(null);
      setLoading(false);
      setError(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);

    apiJson<T>(url)
      .then((result) => {
        if (!cancelled && mountedRef.current) {
          setData(result);
          setError(null);
        }
      })
      .catch((err) => {
        if (!cancelled && mountedRef.current) {
          setError(err instanceof ApiRequestError ? err : new ApiRequestError("Request failed"));
        }
      })
      .finally(() => {
        if (!cancelled && mountedRef.current) {
          setLoading(false);
        }
      });

    return () => { cancelled = true; };
  }, [serialized]);

  const refetch = useCallback(async () => {
    if (url == null) return;
    setFetchId((id) => id + 1);
  }, [url]);

  return { data, loading, error, refetch };
}
