import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { usePreflight } from "@/features/scheduling/hooks/usePreflight";
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
    expect(result.current.lastParams).toBeNull();
  });

  it("check() sets loading and checking status", async () => {
    mockApiJson.mockImplementation(() => new Promise(() => {}));
    const { result } = renderHook(() => usePreflight());

    act(() => {
      result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.loading).toBe(true);
    expect(result.current.status).toBe("checking");
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

  it("409 with conflict details sets blocked", async () => {
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

  it("500 db_error sets error status", async () => {
    const err = new ApiRequestError("Database error", { status: 500, code: "db_error" });
    err.details = { error: "internal" };
    mockApiJson.mockRejectedValue(err);

    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.status).toBe("error");
    expect(result.current.details).toBeNull();
    expect(result.current.error).toBe(err);
  });

  it("504 timeout sets error status", async () => {
    const err = new ApiRequestError("Gateway timeout", { status: 504, code: "timeout" });
    mockApiJson.mockRejectedValue(err);

    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.status).toBe("error");
    expect(result.current.details).toBeNull();
    expect(result.current.error).toBe(err);
  });

  it("Network Error sets error status", async () => {
    mockApiJson.mockRejectedValue(new Error("Network error"));

    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.status).toBe("error");
    expect(result.current.details).toBeNull();
    expect(result.current.error).toBeInstanceOf(ApiRequestError);
  });

  it("409 without conflict details sets error status", async () => {
    const err = new ApiRequestError("Conflict", { status: 409, code: "conflict" });
    // details is not valid ConflictDetails — missing conflicts, requested
    err.details = { foo: "bar" };
    mockApiJson.mockRejectedValue(err);

    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    expect(result.current.status).toBe("error");
    expect(result.current.details).toBeNull();
    expect(result.current.error).toBe(err);
  });

  it("reset() returns to idle and aborts active request", async () => {
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
    expect(result.current.lastParams).toBeNull();
  });

  it("reset() ignores a stale in-flight response", async () => {
    let resolveFlight!: (v: unknown) => void;
    mockApiJson.mockImplementation(() => new Promise((resolve) => { resolveFlight = resolve; }));
    const { result } = renderHook(() => usePreflight());

    act(() => {
      void result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });
    expect(result.current.loading).toBe(true);

    act(() => {
      result.current.reset();
    });

    expect(result.current.status).toBe("idle");
    expect(result.current.loading).toBe(false);

    await act(async () => {
      resolveFlight({ status: "available" });
    });

    // reset aborted the controller, so resolved response is discarded
    expect(result.current.status).toBe("idle");
    expect(result.current.loading).toBe(false);
    expect(result.current.details).toBeNull();
  });

  it("aborted obsolete request does not show error", async () => {
    let resolveA!: (v: unknown) => void;
    mockApiJson
      .mockImplementationOnce(() => new Promise((resolve) => { resolveA = resolve; }))
      .mockImplementationOnce(() => new Promise(() => {}));

    const { result } = renderHook(() => usePreflight());

    act(() => {
      void result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "", end_at: "" });
    });

    // Second check aborts the first
    act(() => {
      void result.current.check({ course_id: "c2", teacher_id: "t2", room_id: null, start_at: "", end_at: "" });
    });

    // Resolve the first (now aborted) check
    await act(async () => {
      resolveA({ status: "available" });
    });

    // First check was aborted — state should be checking (B in-flight)
    expect(result.current.status).toBe("checking");
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(true);
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

    // Resolve A (stale) after B is in-flight — A was aborted so is discarded
    await act(async () => {
      resolveA({ status: "provisional" });
    });

    // B is still in-flight so status is checking
    expect(result.current.status).toBe("checking");
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

  it("series preflight includes series_id when supplied", async () => {
    mockApiJson.mockResolvedValue({ status: "available", occurrences_planned: 3 });
    const { result } = renderHook(() => usePreflight("preflight_series"));

    await act(async () => {
      await result.current.check({
        series_id: "series-1",
        course_id: "course-1",
        teacher_id: "teacher-1",
        room_id: null,
        weekdays: [1],
        start_local_time: "09:00",
        duration_minutes: 60,
        start_date: "2026-08-03",
        count: 10,
        start_at: "",
        end_at: "",
      });
    });

    expect(JSON.parse(mockApiJson.mock.calls[0][1].body).series_id).toBe("series-1");
  });

  it("check sets lastParams", async () => {
    const params = { course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-01T00:00:00Z", end_at: "2024-01-01T01:00:00Z" };
    mockApiJson.mockResolvedValue({ status: "available" });
    const { result } = renderHook(() => usePreflight());

    await act(async () => {
      await result.current.check(params);
    });

    expect(result.current.lastParams).toEqual(params);
  });

  it("classifies an unknown thrown value as an error", async () => {
    mockApiJson.mockRejectedValue("network failure");
    const { result } = renderHook(() => usePreflight());
    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-01T00:00:00Z", end_at: "2024-01-01T01:00:00Z" });
    });
    expect(result.current.status).toBe("error");
    expect(result.current.error?.message).toBe("Unknown error");
  });

  it("sends explicit student include and exclude overrides", async () => {
    mockApiJson.mockResolvedValue({ status: "available" });
    const { result } = renderHook(() => usePreflight());
    await act(async () => {
      await result.current.check({ course_id: "c1", teacher_id: "t1", room_id: null, start_at: "2024-01-01T00:00:00Z", end_at: "2024-01-01T01:00:00Z", included_student_ids: ["s1"], excluded_student_ids: ["s2"] });
    });
    const body = JSON.parse(mockApiJson.mock.calls.at(-1)?.[1].body as string);
    expect(body.included_student_ids).toEqual(["s1"]);
    expect(body.excluded_student_ids).toEqual(["s2"]);
  });
});
