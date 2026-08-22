import { useState, useCallback, useRef } from "react";
import { type QueryKey } from "@tanstack/react-query";
import { ApiRequestError, apiJson } from "@/api/client";
import { queryClient } from "@/query/cache";

export type UseApiMutationResult<TBody, TResp> = {
  mutate: (body: TBody, url: string) => Promise<TResp>;
  loading: boolean;
  error: ApiRequestError | null;
  reset: () => void;
};

/**
 * Bare mutation wrapper with optional cache invalidation. When `invalidate`
 * keys are set, a successful mutation marks those query-key roots stale so
 * every cached variant (filters, pagination) reconciles with server truth
 * instead of only the current observer refetching.
 */
export function useApiMutation<TBody, TResp = unknown>(
  method: string,
  options?: { invalidate?: readonly QueryKey[] },
): UseApiMutationResult<TBody, TResp> {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<ApiRequestError | null>(null);
  const mountedRef = useRef(true);
  // Ref keeps `mutate` referentially stable when callers pass inline arrays.
  const invalidateRef = useRef(options?.invalidate);
  invalidateRef.current = options?.invalidate;

  const reset = useCallback(() => {
    setLoading(false);
    setError(null);
  }, []);

  const mutate = useCallback(async (body: TBody, url: string): Promise<TResp> => {
    setLoading(true);
    setError(null);
    const invalidate = invalidateRef.current;

    try {
      const result = await apiJson<TResp>(url, {
        method,
        body: JSON.stringify(body),
      });
      if (invalidate?.length) {
        await Promise.all(
          invalidate.map((queryKey) => queryClient.invalidateQueries({ queryKey, refetchType: "active" })),
        );
      }
      if (mountedRef.current) {
        setLoading(false);
      }
      return result;
    } catch (err) {
      if (mountedRef.current) {
        setLoading(false);
        if (err instanceof ApiRequestError) {
          setError(err);
        } else {
          setError(new ApiRequestError("Request failed"));
        }
      }
      throw err;
    }
  }, [method]);

  return { mutate, loading, error, reset };
}
