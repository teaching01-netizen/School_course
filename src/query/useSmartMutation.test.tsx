import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { queryClient, queryKeys } from "./cache";
import { useOperationalQuery } from "./useOperationalQuery";
import { useSmartMutation } from "./useSmartMutation";

const mockApiJson = vi.hoisted(() => vi.fn());
vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

type Item = { id: string; status: string };
type Page = { items: Item[]; total: number };

const page = (items: Item[]): Page => ({ items, total: items.length });

function setListData(request: string, data: Page) {
  queryClient.setQueryData<Page>(queryKeys.absences.list(request), data);
}

async function renderActiveList(request: string) {
  const hook = renderHook(() =>
    useOperationalQuery<Page>(queryKeys.absences.list(request), `/api/v1/absences?${request}`),
  );
  await waitFor(() => expect(hook.result.current.isSuccess).toBe(true));
  return hook;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("useSmartMutation", () => {
  beforeEach(() => {
    queryClient.clear();
    mockApiJson.mockReset();
  });

  it("applies the optimistic patch to every cached variant before the request resolves", async () => {
    setListData("status=pending", page([{ id: "a", status: "pending" }]));
    setListData("status=all", page([{ id: "a", status: "pending" }, { id: "b", status: "reviewed" }]));
    const pending = deferred<{ status: string }>();

    const { result } = renderHook(() =>
      useSmartMutation<{ id: string; status: string }, { status: string }>({
        mutationFn: () => pending.promise,
        optimistic: (vars) => [
          {
            keyPrefix: queryKeys.absences.all,
            patch: (data) => ({
              ...(data as Page),
              items: (data as Page).items.map((item) =>
                item.id === vars.id ? { ...item, status: vars.status } : item,
              ),
            }),
          },
        ],
      }),
    );

    let settled = false;
    void result.current.mutateAsync({ id: "a", status: "reviewed" }).then(() => {
      settled = true;
    });

    await waitFor(() => {
      expect(queryClient.getQueryData(queryKeys.absences.list("status=pending"))).toEqual(
        page([{ id: "a", status: "reviewed" }]),
      );
      expect(queryClient.getQueryData(queryKeys.absences.list("status=all"))).toEqual(
        page([{ id: "a", status: "reviewed" }, { id: "b", status: "reviewed" }]),
      );
    });
    expect(settled).toBe(false);
  });

  it("restores the exact snapshot when the request fails", async () => {
    const original = page([{ id: "a", status: "pending" }]);
    setListData("status=pending", original);
    const failure = deferred<never>();

    const { result } = renderHook(() =>
      useSmartMutation<{ id: string }, never>({
        mutationFn: () => failure.promise,
        optimistic: (vars) => [
          {
            keyPrefix: queryKeys.absences.all,
            patch: (data) => ({
              ...(data as Page),
              items: (data as Page).items.map((item) => (item.id === vars.id ? { ...item, status: "gone" } : item)),
            }),
          },
        ],
      }),
    );

    const caught = result.current.mutateAsync({ id: "a" }).catch(() => "errored");
    await waitFor(() =>
      expect(queryClient.getQueryData(queryKeys.absences.list("status=pending"))).toEqual(
        page([{ id: "a", status: "gone" }]),
      ),
    );

    await act(async () => failure.reject(new Error("boom")));
    await expect(caught).resolves.toBe("errored");
    expect(queryClient.getQueryData(queryKeys.absences.list("status=pending"))).toEqual(original);
  });

  it("invalidates the declared roots after settle so active queries refetch", async () => {
    mockApiJson.mockResolvedValue(page([{ id: "a", status: "pending" }]));
    const request = "status=pending";
    await renderActiveList(request);
    expect(mockApiJson).toHaveBeenCalledTimes(1);

    renderHook(() =>
      useSmartMutation<{ id: string }, { status: string }>({
        mutationFn: async () => ({ status: "reviewed" }),
        optimistic: (vars) => [
          {
            keyPrefix: queryKeys.absences.all,
            patch: (data) => ({
              ...(data as Page),
              items: (data as Page).items.map((item) => (item.id === vars.id ? { ...item, status: "reviewed" } : item)),
            }),
          },
        ],
        invalidates: [queryKeys.absences.all],
      }),
    ).result.current.mutateAsync({ id: "a" });

    await waitFor(() => expect(mockApiJson).toHaveBeenCalledTimes(2));
  });

  it("an error rollback only undoes that mutation's changes, not overlapping in-flight ones", async () => {
    setListData("status=pending", page([{ id: "a", status: "pending" }]));
    const first = deferred<{ ok: true }>();
    const second = deferred<never>();

    const { result } = renderHook(() =>
      useSmartMutation<{ id: string; status: string }, unknown>({
        mutationFn: (vars) => (vars.status === "reviewed" ? first.promise : second.promise),
        optimistic: (vars) => [
          {
            keyPrefix: queryKeys.absences.all,
            patch: (data) => ({
              ...(data as Page),
              items: (data as Page).items.map((item) =>
                item.id === vars.id ? { ...item, status: vars.status } : item,
              ),
            }),
          },
        ],
      }),
    );

    const review = result.current.mutateAsync({ id: "a", status: "reviewed" });
    const cancel = result.current.mutateAsync({ id: "a", status: "cancelled" }).catch(() => "errored");
    await waitFor(() =>
      expect(queryClient.getQueryData(queryKeys.absences.list("status=pending"))).toEqual(
        page([{ id: "a", status: "cancelled" }]),
      ),
    );

    await act(async () => {
      first.resolve({ ok: true });
      second.reject(new Error("boom"));
    });
    await review;
    await expect(cancel).resolves.toBe("errored");
    // The failed cancel rolls back to its snapshot (which included the review),
    // so the succeeded review survives.
    expect(queryClient.getQueryData(queryKeys.absences.list("status=pending"))).toEqual(
      page([{ id: "a", status: "reviewed" }]),
    );
  });
});
