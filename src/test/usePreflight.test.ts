import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { usePreflight } from "@/hooks/usePreflight";
import { ApiRequestError } from "@/api/client";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

describe("usePreflight", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
  });

  it("initial state is idle", () => {
    const { result } = renderHook(() => usePreflight());
    expect(result.current.status).toBe("idle");
    expect(result.current.loading).toBe(false);
    expect(result.current.details).toBeNull();
    expect(result.current.error).toBeNull();
    expect(result.current.occurrencesPlanned).toBeNull();
  });

  it("check() sets loading", async () => {
    mockApiJson.mockImplementation(() => new Promise(() => {}));
    const { result } = renderHook(() => usePreflight());

    act(() => {
      result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.loading).toBe(true);
  });

  it("successful API call sets available", async () => {
    mockApiJson.mockResolvedValue({ status: "available" });
    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-01T00:00:00Z", end_at: "2024-01-01T01:00:00Z" });
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.status).toBe("available");
    expect(result.current.error).toBeNull();
  });

  it("successful API call sets provisional", async () => {
    mockApiJson.mockResolvedValue({ status: "provisional" });
    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.status).toBe("provisional");
  });

  it("API error sets blocked with conflict details", async () => {
    const conflictDetails = { kind: "room_overlap", requested: { start_at: "", end_at: "", course_id: "c1", room_id: null, teacher_id: "t1" }, conflicts: [] };
    const err = new ApiRequestError("Conflict", { status: 409, code: "conflict" });
    err.details = conflictDetails;
    mockApiJson.mockRejectedValue(err);

    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.status).toBe("blocked");
    expect(result.current.details).toEqual(conflictDetails);
    expect(result.current.error).toBe(err);
  });

  it("non-API error sets blocked without details", async () => {
    mockApiJson.mockRejectedValue(new Error("Network error"));

    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.status).toBe("blocked");
    expect(result.current.details).toBeNull();
    expect(result.current.error).toBeInstanceOf(Error);
  });

  it("reset() returns to idle", async () => {
    mockApiJson.mockResolvedValue({ status: "available" });
    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });
    expect(result.current.status).toBe("available");

    act(() => { result.current.reset(); });

    expect(result.current.status).toBe("idle");
    expect(result.current.details).toBeNull();
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it("uses preflight_series endpoint when specified", async () => {
    mockApiJson.mockResolvedValue({ status: "available", occurrences_planned: 5 });
    const { result } = renderHook(() => usePreflight("preflight_series"));

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/scheduling/preflight_series",
      expect.objectContaining({ method: "POST" })
    );
    expect(result.current.occurrencesPlanned).toBe(5);
  });

  it("last check() result wins even if earlier check resolves later", async () => {
    let resolveA!: (v: unknown) => void;
    let resolveB!: (v: unknown) => void;
    const callOrder: string[] = [];
    mockApiJson
      .mockImplementationOnce(async () => {
        callOrder.push("A called");
        return new Promise((r) => { resolveA = r; });
      })
      .mockImplementationOnce(async () => {
        callOrder.push("B called");
        return new Promise((r) => { resolveB = r; });
      });

    const { result } = renderHook(() => usePreflight());

    act(() => { void result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-01T00:00:00Z", end_at: "2024-01-01T01:00:00Z" }); });
    act(() => { void result.current.check({ course_id: "c2", teacher_id: "t2", room_id: null, start_at: "2024-01-02T00:00:00Z", end_at: "2024-01-02T01:00:00Z" }); });

    // Resolve A (stale) after B is in-flight
    await act(async () => {
      resolveA({ status: "provisional" });
    });

    // Status should still be idle — B is still in-flight
    expect(result.current.status).toBe("idle");
    expect(result.current.loading).toBe(true);

    // Now resolve B
    await act(async () => {
      resolveB({ status: "available" });
    });

    expect(result.current.status).toBe("available");
    expect(result.current.loading).toBe(false);
  });

  it("series preflight body includes weekdays and duration", async () => {
    mockApiJson.mockResolvedValue({ status: "available", occurrences_planned: 3 });
    const { result } = renderHook(() => usePreflight("preflight_series"));

    await act(async () => {
      await result.current.check({
        course_id: "c1",
        teacher_id: "t1",
        room_id: null,
        start_at: "",
        end_at: "",
        weekdays: [1, 3],
        start_local_time: "09:00",
        duration_minutes: 60,
        start_date: "2024-01-01",
        end_date: "2024-06-01",
        count: null,
      });
    });

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/scheduling/preflight_series",
      expect.objectContaining({
        body: expect.stringContaining("weekdays"),
      })
    );
    const body = JSON.parse(mockApiJson.mock.calls[0][1].body);
    expect(body.weekdays).toEqual([1, 3]);
    expect(body.duration_minutes).toBe(60);
    expect(body.count).toBeNull();
  });
});
