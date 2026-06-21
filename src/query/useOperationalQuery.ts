import { keepPreviousData, useQuery, type QueryKey } from "@tanstack/react-query";
import { ApiRequestError, apiJson } from "@/api/client";
import { cachePolicies, queryClient } from "./cache";

export function useOperationalQuery<T>(queryKey: QueryKey, url: string | null) {
  return useQuery<T, ApiRequestError>({
    queryKey,
    queryFn: () => apiJson<T>(url!, { method: "GET" }),
    enabled: url != null,
    ...cachePolicies.operational,
    placeholderData: keepPreviousData,
  }, queryClient);
}
