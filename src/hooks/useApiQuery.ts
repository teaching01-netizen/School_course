import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ApiRequestError, apiJson } from "@/api/client";
import { cachePolicies, cachePolicyForURL, queryClient, queryKeyForURL } from "@/query/cache";

export type UseApiQueryResult<T> = {
  data: T | null;
  loading: boolean;
  refreshing: boolean;
  error: ApiRequestError | null;
  refetch: () => Promise<void>;
};

export function useApiQuery<T>(url: string | null, deps?: unknown[], options?: { keepPreviousData?: boolean }): UseApiQueryResult<T> {
  const policy = url ? cachePolicyForURL(url) : cachePolicies.semiStatic;
  const operational = policy === cachePolicies.operational;
  const query = useQuery<T, ApiRequestError>({
    queryKey: url ? queryKeyForURL(url, deps) : ["api", "disabled"],
    queryFn: () => apiJson<T>(url!),
    enabled: url != null,
    ...policy,
    placeholderData: operational || options?.keepPreviousData ? keepPreviousData : undefined,
  }, queryClient);

  return {
    data: query.data ?? null,
    loading: url != null && query.isPending,
    refreshing: url != null && query.isFetching && !query.isPending,
    error: query.error ?? null,
    refetch: async () => {
      if (url == null) return;
      await query.refetch();
    },
  };
}
