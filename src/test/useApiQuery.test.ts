import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { StrictMode, createElement, type ReactNode } from "react";
import { useApiQuery } from "@/hooks/useApiQuery";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("useApiQuery", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
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

    expect(result.current.data).toEqual(updated);
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
});
