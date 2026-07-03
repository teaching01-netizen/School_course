import { describe, it, expect, vi } from "vitest";
import usePreflightGate from "@/features/scheduling/hooks/usePreflightGate";
import type { UsePreflightReturn } from "@/features/scheduling/hooks/usePreflight";

function makePreflight(overrides: Partial<UsePreflightReturn>): UsePreflightReturn {
  return {
    status: "idle",
    loading: false,
    details: null,
    error: null,
    occurrencesPlanned: null,
    check: vi.fn(),
    reset: vi.fn(),
    ...overrides,
  };
}

describe("usePreflightGate", () => {
  it("blocks save when required fields are missing even if preflight is available", () => {
    const gate = usePreflightGate(makePreflight({ status: "available" }), {
      requiredFields: ["course-1", "", "09:00"],
    });

    expect(gate.canSave).toBe(false);
    expect(gate.reason).toBe("no_fields");
  });

  it("allows save only when required fields are filled and preflight passes", () => {
    const gate = usePreflightGate(makePreflight({ status: "provisional" }), {
      requiredFields: ["course-1", "teacher-1", "09:00"],
    });

    expect(gate.canSave).toBe(true);
    expect(gate.reason).toBe("ok");
  });
});
