import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useApiMutation } from "@/hooks/useApiMutation";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("useApiMutation", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("mutate sets loading, returns response on success", async () => {
    const resp = { id: "42" };
    mockApiJson.mockResolvedValue(resp);

    const { result } = renderHook(() => useApiMutation("POST"));

    let promise: Promise<unknown>;
    act(() => {
      promise = result.current.mutate({ name: "new" }, "/api/v1/test");
    });

    expect(result.current.loading).toBe(true);

    await act(async () => {
      await promise;
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
    expect(mockApiJson).toHaveBeenCalledWith("/api/v1/test", {
      method: "POST",
      body: JSON.stringify({ name: "new" }),
    });
  });

  it("mutate sets error on failure", async () => {
    const err = new ApiRequestError("Bad request", { status: 400, code: "bad_input" });
    mockApiJson.mockRejectedValue(err);

    const { result } = renderHook(() => useApiMutation("DELETE"));

    await act(async () => {
      try {
        await result.current.mutate({}, "/api/v1/test/1");
      } catch {
        // expected
      }
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBe(err);
  });

  it("reset clears error", async () => {
    const err = new ApiRequestError("Bad request", { status: 400 });
    mockApiJson.mockRejectedValue(err);

    const { result } = renderHook(() => useApiMutation("POST"));

    await act(async () => {
      try {
        await result.current.mutate({}, "/api/v1/test");
      } catch {
        // expected
      }
    });

    expect(result.current.error).toBe(err);

    act(() => {
      result.current.reset();
    });

    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it("mutate preserves previous error state until next call", async () => {
    const err = new ApiRequestError("Fail", { status: 500 });
    mockApiJson.mockRejectedValueOnce(err);

    const { result } = renderHook(() => useApiMutation("POST"));

    await act(async () => {
      try {
        await result.current.mutate({}, "/api/v1/test");
      } catch {
        // expected
      }
    });

    expect(result.current.error).toBe(err);

    mockApiJson.mockResolvedValueOnce({ ok: true });

    await act(async () => {
      await result.current.mutate({}, "/api/v1/test");
    });

    expect(result.current.error).toBeNull();
  });
});
