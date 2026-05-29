import { useState, useCallback, useRef } from "react";
import { ApiRequestError, apiJson } from "@/api/client";

export type UseApiMutationResult<TBody, TResp> = {
  mutate: (body: TBody, url: string) => Promise<TResp>;
  loading: boolean;
  error: ApiRequestError | null;
  reset: () => void;
};

export function useApiMutation<TBody, TResp = unknown>(method: string): UseApiMutationResult<TBody, TResp> {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<ApiRequestError | null>(null);
  const mountedRef = useRef(true);

  const reset = useCallback(() => {
    setLoading(false);
    setError(null);
  }, []);

  const mutate = useCallback(async (body: TBody, url: string): Promise<TResp> => {
    setLoading(true);
    setError(null);

    try {
      const result = await apiJson<TResp>(url, {
        method,
        body: JSON.stringify(body),
      });
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
