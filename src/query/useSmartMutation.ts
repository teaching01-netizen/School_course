import { useMutation, type QueryKey } from "@tanstack/react-query";
import { queryClient } from "./cache";

/**
 * A mutation that participates in the query cache as a transaction:
 *
 *   cancel in-flight refetches -> snapshot -> patch cache (UI updates instantly)
 *     -> server call -> invalidate roots (server truth reconciles silently)
 *     -> on error: restore the exact snapshot
 *
 * The optimistic patch only needs to cover the network latency window; the
 * post-settle invalidation (plus the realtime bridge / polling for operational
 * data) guarantees convergence to server truth.
 */
export type OptimisticEffect<TVars> = {
  /** Patch every cached query whose key starts with this prefix. */
  keyPrefix: QueryKey;
  /** Pure transform of the cached value. Must not mutate its input. */
  patch: (data: unknown, vars: TVars) => unknown;
};

export type SmartMutationConfig<TVars, TResp> = {
  mutationFn: (vars: TVars) => Promise<TResp>;
  /** Cache effects applied instantly while the request is in flight. */
  optimistic?: (vars: TVars) => readonly OptimisticEffect<TVars>[];
  /** Query-key roots invalidated after settle so server truth reconciles. */
  invalidates?: readonly QueryKey[];
  onSuccess?: (data: TResp, vars: TVars) => void | Promise<void>;
  onError?: (error: unknown, vars: TVars) => void | Promise<void>;
};

type Snapshot = { key: QueryKey; data: unknown }[];

export function useSmartMutation<TVars, TResp = unknown>(config: SmartMutationConfig<TVars, TResp>) {
  return useMutation<TResp, unknown, TVars, { snapshot: Snapshot }>(
    {
      mutationFn: config.mutationFn,
      onMutate: async (vars) => {
        const effects = config.optimistic?.(vars) ?? [];
        const prefixes = [...new Set(effects.map((effect) => effect.keyPrefix))];
        // Stop refetches from clobbering the optimistic state mid-flight.
        await Promise.all(prefixes.map((keyPrefix) => queryClient.cancelQueries({ queryKey: keyPrefix })));
        const snapshot: Snapshot = prefixes.flatMap((keyPrefix) =>
          queryClient.getQueriesData({ queryKey: keyPrefix }).map(([key, data]) => ({ key, data })),
        );
        for (const { keyPrefix, patch } of effects) {
          for (const [key, data] of queryClient.getQueriesData({ queryKey: keyPrefix })) {
            queryClient.setQueryData(key, patch(data, vars));
          }
        }
        return { snapshot };
      },
      onError: (error, vars, context) => {
        for (const { key, data } of context?.snapshot ?? []) queryClient.setQueryData(key, data);
        void config.onError?.(error, vars);
      },
      onSuccess: (data, vars) => {
        void config.onSuccess?.(data, vars);
      },
      onSettled: async () => {
        await Promise.all(
          (config.invalidates ?? []).map((queryKey) =>
            queryClient.invalidateQueries({ queryKey, refetchType: "active" }),
          ),
        );
      },
    },
    queryClient,
  );
}

/**
 * Copy of a `{ items }` page with `update` applied to each item; returning
 * `null` from `update` removes the item. Values that do not look like an item
 * page pass through unchanged, so key-prefix patching stays safe across
 * heterogeneous cached entries.
 */
export function mapPageItems<Page extends { items: Item[] }, Item>(
  page: unknown,
  update: (item: Item) => Item | null,
): unknown {
  if (page == null || typeof page !== "object" || !Array.isArray((page as { items?: unknown }).items)) return page;
  const typed = page as Page;
  const items: Item[] = [];
  for (const item of typed.items) {
    const next = update(item);
    if (next !== null) items.push(next);
  }
  return { ...typed, items };
}

/**
 * Same contract as {@link mapPageItems} for caches that store a bare item
 * array (e.g. session lists) instead of an `{ items }` page.
 */
export function mapListItems<Item>(
  data: unknown,
  update: (item: Item) => Item | null,
): unknown {
  if (!Array.isArray(data)) return data;
  const items: Item[] = [];
  for (const item of data as Item[]) {
    const next = update(item);
    if (next !== null) items.push(next);
  }
  return items;
}
