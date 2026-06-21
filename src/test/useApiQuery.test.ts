import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { StrictMode, createElement, type ReactNode } from "react";
import { useApiQuery } from "@/hooks/useApiQuery";
import { ApiRequestError } from "@/api/client";
import { QueryClientProvider } from "@tanstack/react-query";
import { createAppQueryClient, queryClient } from "@/query/cache";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("useApiQuery", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    queryClient.clear();
  });

  it("returns loading=true on mount", () => {
    mockApiJson.mockImplementation(() => new Promise(() => {}));
    const { result } = renderHook(() => useApiQuery("/api/v1/test"));
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it("returns data after successful fetch", async () => {
    const data = [{ id: "1", name: "test" }];
    mockApiJson.mockResolvedValue(data);

    const { result } = renderHook(() => useApiQuery("/api/v1/test"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toEqual(data);
    expect(result.current.error).toBeNull();
  });

  it("returns error after failed fetch", async () => {
    const err = new ApiRequestError("Not found", { status: 404 });
    mockApiJson.mockRejectedValue(err);

    const { result } = renderHook(() => useApiQuery("/api/v1/test"));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBe(err);
  });

  it("refetch() re-fetches and returns updated data", async () => {
    const initial = [{ id: "1" }];
    const updated = [{ id: "2" }];
    mockApiJson.mockResolvedValueOnce(initial);

    const { result } = renderHook(() => useApiQuery("/api/v1/test"));

    await waitFor(() => expect(result.current.data).toEqual(initial));

    mockApiJson.mockResolvedValueOnce(updated);

    await act(async () => {
      await result.current.refetch();
    });

    await waitFor(() => expect(result.current.data).toEqual(updated));
    expect(result.current.loading).toBe(false);
  });

  it("returns null data when url is null", () => {
    const { result } = renderHook(() => useApiQuery(null));

    expect(result.current.loading).toBe(false);
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it("re-fetches when url changes", async () => {
    const data1 = [{ id: "1" }];
    const data2 = [{ id: "2" }];
    mockApiJson.mockResolvedValueOnce(data1).mockResolvedValueOnce(data2);

    const { result, rerender } = renderHook(({ url }) => useApiQuery(url), {
      initialProps: { url: "/api/v1/a" },
    });

    await waitFor(() => expect(result.current.data).toEqual(data1));

    rerender({ url: "/api/v1/b" });

    await waitFor(() => expect(result.current.data).toEqual(data2));
  });

  it("distinguishes retained operational data from initial loading", async () => {
    let resolveNext: (value: Array<{ id: string }>) => void = () => undefined;
    mockApiJson
      .mockResolvedValueOnce([{ id: "old" }])
      .mockImplementationOnce(() => new Promise((resolve) => { resolveNext = resolve; }));

    const { result, rerender } = renderHook(({ month }) => useApiQuery(
      `/api/v1/teacher/dashboard?month_start=${month}`,
    ), { initialProps: { month: "2026-06-01" } });
    await waitFor(() => expect(result.current.data).toEqual([{ id: "old" }]));

    rerender({ month: "2026-07-01" });

    expect(result.current.data).toEqual([{ id: "old" }]);
    expect(result.current.loading).toBe(false);
    expect(result.current.refreshing).toBe(true);

    await act(async () => resolveNext([{ id: "new" }]));
    await waitFor(() => expect(result.current.data).toEqual([{ id: "new" }]));
    expect(result.current.refreshing).toBe(false);
  });

  it("clears stale data when a new url fails", async () => {
    const data1 = [{ id: "1" }];
    const err = new ApiRequestError("Server down", { status: 500 });
    mockApiJson.mockResolvedValueOnce(data1).mockRejectedValue(err);

    const { result, rerender } = renderHook(({ url }) => useApiQuery(url), {
      initialProps: { url: "/api/v1/a" },
    });

    await waitFor(() => expect(result.current.data).toEqual(data1));

    rerender({ url: "/api/v1/b" });

    await waitFor(() => expect(result.current.error).toBe(err), { timeout: 2_500 });
    expect(result.current.data).toBeNull();
  });

  it("does not call apiJson after unmount", async () => {
    mockApiJson.mockImplementation(() => new Promise((resolve) => setTimeout(() => resolve([{ id: "1" }]), 100)));

    const { unmount } = renderHook(() => useApiQuery("/api/v1/test"));
    unmount();

    await new Promise((r) => setTimeout(r, 150));
    // If no error thrown, test passes
  });

  it("resolves loading in StrictMode", async () => {
    const data = [{ id: "1", name: "strict" }];
    mockApiJson.mockResolvedValue(data);

    const wrapper = ({ children }: { children: ReactNode }) => createElement(StrictMode, null, children);
    const { result } = renderHook(() => useApiQuery("/api/v1/test"), { wrapper });

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toEqual(data);
    expect(result.current.error).toBeNull();
  });

  it("deduplicates identical requests through the shared query cache", async () => {
    const client = createAppQueryClient();
    mockApiJson.mockResolvedValue([{ id: "shared" }]);
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client }, children);

    const first = renderHook(() => useApiQuery("/api/v1/subjects"), { wrapper });
    const second = renderHook(() => useApiQuery("/api/v1/subjects"), { wrapper });

    await waitFor(() => expect(first.result.current.data).toEqual([{ id: "shared" }]));
    await waitFor(() => expect(second.result.current.data).toEqual([{ id: "shared" }]));
    expect(mockApiJson).toHaveBeenCalledTimes(1);
  });
});
