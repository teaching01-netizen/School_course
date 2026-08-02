import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useDebouncedPreflight } from "./useDebouncedPreflight";
import type { UsePreflightReturn, PreflightParams } from "./usePreflight";

function makePreflight(): UsePreflightReturn {
  return {
    status: "idle" as const,
    loading: false,
    details: null,
    error: null,
    occurrencesPlanned: null,
    lastParams: null,
    check: vi.fn(),
    reset: vi.fn(),
  };
}

const sampleParams: PreflightParams = {
  course_id: "c1",
  teacher_id: "t1",
  room_id: null,
  start_at: "2024-01-01T00:00:00Z",
  end_at: "2024-01-01T01:00:00Z",
};

describe("useDebouncedPreflight", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("calls preflight.check once after delay when params change rapidly", () => {
    const preflight = makePreflight();

    const { rerender, unmount } = renderHook(
      ({ params }) => useDebouncedPreflight(preflight, params, { enabled: true, delayMs: 300 }),
      { initialProps: { params: sampleParams } },
    );

    // No call immediately
    expect(preflight.check).not.toHaveBeenCalled();

    // Five rapid changes within 200ms — only last params should fire
    for (let i = 0; i < 5; i++) {
      rerender({
        params: {
          ...sampleParams,
          room_id: `r${i}`,
        },
      });
      vi.advanceTimersByTime(40); // 5 × 40 = 200ms, still inside debounce window
    }

    // Still no call
    expect(preflight.check).not.toHaveBeenCalled();

    // Advance 300ms from last change (200ms elapsed, so 300ms more = 500ms total)
    vi.advanceTimersByTime(300);

    expect(preflight.check).toHaveBeenCalledTimes(1);
    expect(preflight.check).toHaveBeenCalledWith({
      ...sampleParams,
      room_id: "r4",
    });

    unmount();
  });

  it("does not fire before the delay elapses", () => {
    const preflight = makePreflight();

    const { unmount } = renderHook(
      () => useDebouncedPreflight(preflight, sampleParams, { enabled: true, delayMs: 300 }),
    );

    vi.advanceTimersByTime(299);
    expect(preflight.check).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(preflight.check).toHaveBeenCalledTimes(1);

    unmount();
  });

  it("new params abort the previous debounce timer", () => {
    const preflight = makePreflight();

    const { rerender, unmount } = renderHook(
      ({ params }) => useDebouncedPreflight(preflight, params, { enabled: true, delayMs: 300 }),
      { initialProps: { params: sampleParams } },
    );

    // Start first debounce
    vi.advanceTimersByTime(200);

    // Change params — resets timer
    rerender({
      params: { ...sampleParams, room_id: "r1" },
    });

    // Only 200ms elapsed from second change — not enough
    vi.advanceTimersByTime(200);
    expect(preflight.check).not.toHaveBeenCalled();

    // Now advance to 300ms from second change
    vi.advanceTimersByTime(100);
    expect(preflight.check).toHaveBeenCalledTimes(1);
    expect(preflight.check).toHaveBeenCalledWith({
      ...sampleParams,
      room_id: "r1",
    });

    unmount();
  });

  it("unmount clears pending timer and does not fire", () => {
    const preflight = makePreflight();

    const { unmount } = renderHook(
      () => useDebouncedPreflight(preflight, sampleParams, { enabled: true, delayMs: 300 }),
    );

    vi.advanceTimersByTime(200);
    unmount();

    vi.advanceTimersByTime(100);
    expect(preflight.check).not.toHaveBeenCalled();
  });

  it("clears the prior timer when parameters change", () => {
    const preflight = makePreflight();
    const { rerender, unmount } = renderHook(
      ({ params }) => useDebouncedPreflight(preflight, params, { enabled: true, delayMs: 300 }),
      { initialProps: { params: sampleParams } },
    );
    rerender({ params: { ...sampleParams, room_id: "r1" } });
    unmount();
    expect(preflight.check).not.toHaveBeenCalled();
  });

  it("calls reset when disabled", () => {
    const preflight = makePreflight();

    const { rerender, unmount } = renderHook(
      ({ enabled }) => useDebouncedPreflight(preflight, sampleParams, { enabled, delayMs: 300 }),
      { initialProps: { enabled: true } },
    );

    rerender({ enabled: false });
    expect(preflight.reset).toHaveBeenCalledTimes(1);

    unmount();
  });

  it("clears the scheduled timer when disabled after scheduling", () => {
    const preflight = makePreflight();
    const { rerender, unmount } = renderHook(
      ({ enabled }) => useDebouncedPreflight(preflight, sampleParams, { enabled, delayMs: 300 }),
      { initialProps: { enabled: true } },
    );
    rerender({ enabled: false });
    vi.advanceTimersByTime(300);
    expect(preflight.check).not.toHaveBeenCalled();
    unmount();
  });

  it("calls reset when params become null", () => {
    const preflight = makePreflight();

    let currentParams: PreflightParams | null = sampleParams;
    const { rerender, unmount } = renderHook(
      () => useDebouncedPreflight(preflight, currentParams, { enabled: true, delayMs: 300 }),
    );

    currentParams = null;
    rerender();
    expect(preflight.reset).toHaveBeenCalledTimes(1);

    unmount();
  });
});
