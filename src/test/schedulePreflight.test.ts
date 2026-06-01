import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { usePreflight } from "@/hooks/usePreflight";
import { useState, useCallback } from "react";
import { validateSeriesPreflight, type SeriesPreflightForm } from "@/utils/preflight";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

function useSeriesPreflightWithGuard() {
  const preflight = usePreflight("preflight_series");
  const [form, setForm] = useState<SeriesPreflightForm>({
    course_id: "",
    teacher_id: "",
    weekdays: [false, false, false, false, false, false, false],
    start_local_time: "",
    duration_minutes: 120,
    start_date: "",
    end_date: "",
    room_id: "",
    count: 10,
  });
  const [useCount, setUseCount] = useState(false);

  const updateField = useCallback(<K extends keyof SeriesPreflightForm>(key: K, value: SeriesPreflightForm[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  }, []);

  const runSeriesPreflight = useCallback(async () => {
    const validated = validateSeriesPreflight(form, useCount);
    if (!validated) { preflight.reset(); return; }
    await preflight.check({
      course_id: form.course_id,
      teacher_id: form.teacher_id,
      room_id: validated.room_id,
      weekdays: validated.weekdays,
      start_local_time: form.start_local_time,
      duration_minutes: form.duration_minutes,
      start_date: form.start_date,
      end_date: validated.end_date,
      count: validated.count,
      start_at: "",
      end_at: "",
    });
  }, [form, useCount, preflight]);

  return { preflight, runSeriesPreflight, form, updateField, setUseCount };
}

describe("Schedule create-series preflight guard", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    mockApiJson.mockResolvedValue({ status: "available", occurrences_planned: 5 });
  });

  it("validation fails when required fields are missing", () => {
    const result = validateSeriesPreflight({
      course_id: "", teacher_id: "", room_id: "",
      weekdays: [false, false, false, false, false, false, false],
      start_local_time: "", duration_minutes: 120,
      start_date: "", end_date: "", count: 0,
    }, false);
    expect(result).toBeNull();
  });

  it("preflight fires when all required fields are filled", async () => {
    const { result } = renderHook(() => useSeriesPreflightWithGuard());

    act(() => {
      result.current.updateField("course_id", "course-1");
      result.current.updateField("teacher_id", "teacher-1");
      result.current.updateField("weekdays", [true, false, false, false, false, false, false]);
      result.current.updateField("start_local_time", "09:00");
      result.current.updateField("start_date", "2024-01-01");
      result.current.updateField("end_date", "2024-01-14");
    });

    await act(async () => {
      await result.current.runSeriesPreflight();
    });

    expect(mockApiJson).toHaveBeenCalledTimes(1);
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/scheduling/preflight_series",
      expect.objectContaining({ method: "POST" })
    );

    const body = JSON.parse(mockApiJson.mock.calls[0][1].body);
    expect(body.course_id).toBe("course-1");
    expect(body.teacher_id).toBe("teacher-1");
    expect(body.weekdays).toEqual([0]);
    expect(body.start_local_time).toBe("09:00");
    expect(body.duration_minutes).toBe(120);
    expect(body.start_date).toBe("2024-01-01");
    expect(body.end_date).toBe("2024-01-14");
    expect(body.room_id).toBeNull();
  });

  it("preflight status transitions: idle → loading → available", async () => {
    let resolvePromise!: (v: unknown) => void;
    mockApiJson.mockImplementation(() => new Promise((resolve) => {
      resolvePromise = resolve;
    }));

    const { result } = renderHook(() => useSeriesPreflightWithGuard());

    expect(result.current.preflight.status).toBe("idle");
    expect(result.current.preflight.loading).toBe(false);

    act(() => {
      result.current.updateField("course_id", "course-1");
      result.current.updateField("teacher_id", "teacher-1");
      result.current.updateField("weekdays", [true, false, false, false, false, false, false]);
      result.current.updateField("start_local_time", "09:00");
      result.current.updateField("start_date", "2024-01-01");
      result.current.updateField("end_date", "2024-01-14");
    });

    act(() => {
      result.current.runSeriesPreflight();
    });

    expect(result.current.preflight.loading).toBe(true);

    await act(async () => {
      resolvePromise({ status: "available", occurrences_planned: 5 });
    });

    expect(result.current.preflight.loading).toBe(false);
    expect(result.current.preflight.status).toBe("available");
    expect(result.current.preflight.occurrencesPlanned).toBe(5);
  });

  it("validation fails when a required field is cleared", () => {
    const validResult = validateSeriesPreflight({
      course_id: "course-1", teacher_id: "teacher-1", room_id: "",
      weekdays: [true, false, false, false, false, false, false],
      start_local_time: "09:00", duration_minutes: 120,
      start_date: "2024-01-01", end_date: "2024-01-14", count: 10,
    }, false);
    expect(validResult).not.toBeNull();

    const invalidResult = validateSeriesPreflight({
      course_id: "", teacher_id: "teacher-1", room_id: "",
      weekdays: [true, false, false, false, false, false, false],
      start_local_time: "09:00", duration_minutes: 120,
      start_date: "2024-01-01", end_date: "2024-01-14", count: 10,
    }, false);
    expect(invalidResult).toBeNull();
  });

  it("uses preflight_series endpoint with correct URL and body shape", async () => {
    const { result } = renderHook(() => useSeriesPreflightWithGuard());

    act(() => {
      result.current.updateField("course_id", "c1");
      result.current.updateField("teacher_id", "t1");
      result.current.updateField("weekdays", [false, true, false, true, false, false, false]);
      result.current.updateField("start_local_time", "14:30");
      result.current.updateField("duration_minutes", 90);
      result.current.updateField("start_date", "2024-03-01");
      result.current.updateField("end_date", "2024-06-01");
    });

    await act(async () => {
      await result.current.runSeriesPreflight();
    });

    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/scheduling/preflight_series",
      expect.objectContaining({ method: "POST" })
    );

    const body = JSON.parse(mockApiJson.mock.calls[0][1].body);
    expect(body).toHaveProperty("weekdays");
    expect(Array.isArray(body.weekdays)).toBe(true);
    expect(body.weekdays).toEqual([1, 3]);
    expect(body).toHaveProperty("start_local_time", "14:30");
    expect(body).toHaveProperty("duration_minutes", 90);
    expect(body).toHaveProperty("start_date", "2024-03-01");
    expect(body).toHaveProperty("end_date", "2024-06-01");
    expect(body).toHaveProperty("course_id", "c1");
    expect(body).toHaveProperty("teacher_id", "t1");
  });

  it("useCount mode sends count instead of end_date", async () => {
    const { result } = renderHook(() => useSeriesPreflightWithGuard());

    act(() => {
      result.current.setUseCount(true);
      result.current.updateField("course_id", "course-1");
      result.current.updateField("teacher_id", "teacher-1");
      result.current.updateField("weekdays", [true, false, false, false, false, false, false]);
      result.current.updateField("start_local_time", "09:00");
      result.current.updateField("start_date", "2024-01-01");
      result.current.updateField("count", 8);
    });

    await act(async () => {
      await result.current.runSeriesPreflight();
    });

    expect(mockApiJson).toHaveBeenCalledTimes(1);

    const body = JSON.parse(mockApiJson.mock.calls[0][1].body);
    expect(body.count).toBe(8);
    expect(body.end_date).toBeNull();
  });
});

describe("validateSeriesPreflight edge cases", () => {
  const validForm = {
    course_id: "c1",
    teacher_id: "t1",
    room_id: "",
    weekdays: [true, false, false, false, false, false, false],
    start_local_time: "09:00",
    duration_minutes: 120,
    start_date: "2024-01-01",
    end_date: "2024-01-14",
    count: 10,
  };

  it("returns null when duration_minutes is 0", () => {
    expect(validateSeriesPreflight({ ...validForm, duration_minutes: 0 }, false)).toBeNull();
  });

  it("returns null when duration_minutes is negative", () => {
    expect(validateSeriesPreflight({ ...validForm, duration_minutes: -30 }, false)).toBeNull();
  });

  it("returns null when end_date is empty and useCount is false", () => {
    expect(validateSeriesPreflight({ ...validForm, end_date: "" }, false)).toBeNull();
  });

  it("returns null when count is 0 and useCount is true", () => {
    expect(validateSeriesPreflight({ ...validForm, count: 0 }, true)).toBeNull();
  });

  it("returns null when count is Infinity and useCount is true", () => {
    expect(validateSeriesPreflight({ ...validForm, count: Infinity }, true)).toBeNull();
  });

  it("returns null when count is NaN and useCount is true", () => {
    expect(validateSeriesPreflight({ ...validForm, count: NaN }, true)).toBeNull();
  });

  it("converts empty room_id to null", () => {
    const result = validateSeriesPreflight(validForm, false);
    expect(result).not.toBeNull();
    expect(result!.room_id).toBeNull();
  });

  it("preserves non-empty room_id", () => {
    const result = validateSeriesPreflight({ ...validForm, room_id: "r1" }, false);
    expect(result).not.toBeNull();
    expect(result!.room_id).toBe("r1");
  });

  it("returns null when no weekdays are selected", () => {
    expect(validateSeriesPreflight({
      ...validForm,
      weekdays: [false, false, false, false, false, false, false],
    }, false)).toBeNull();
  });

  it("returns null when start_local_time is empty", () => {
    expect(validateSeriesPreflight({ ...validForm, start_local_time: "" }, false)).toBeNull();
  });

  it("returns null when start_date is empty", () => {
    expect(validateSeriesPreflight({ ...validForm, start_date: "" }, false)).toBeNull();
  });

  it("returns null when course_id is empty", () => {
    expect(validateSeriesPreflight({ ...validForm, course_id: "" }, false)).toBeNull();
  });

  it("returns null when teacher_id is empty", () => {
    expect(validateSeriesPreflight({ ...validForm, teacher_id: "" }, false)).toBeNull();
  });

  it("returns valid result for all valid inputs (useCount=false)", () => {
    const result = validateSeriesPreflight(validForm, false);
    expect(result).toEqual({
      weekdays: [0],
      end_date: "2024-01-14",
      count: null,
      room_id: null,
    });
  });

  it("returns valid result for all valid inputs (useCount=true)", () => {
    const result = validateSeriesPreflight(validForm, true);
    expect(result).toEqual({
      weekdays: [0],
      end_date: null,
      count: 10,
      room_id: null,
    });
  });
});
