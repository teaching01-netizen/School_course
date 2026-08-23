import { describe, it, expect, vi } from "vitest";
import usePreflightGate from "@/features/scheduling/hooks/usePreflightGate";
import type { UsePreflightReturn } from "@/features/scheduling/hooks/usePreflight";

function makePreflight(overrides: Partial<UsePreflightReturn>): UsePreflightReturn {
  return {
    status: "idle",
    loading: false,
    details: null,
    warnings: [],
    error: null,
    occurrencesPlanned: null,
    lastParams: null,
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

  it("blocks save when the form is invalid despite a passing preflight", () => {
    const gate = usePreflightGate(makePreflight({ status: "available" }), { isFormValid: false });
    expect(gate.canSave).toBe(false);
    expect(gate.reason).toBe("idle");
  });

  it.each([
    ["checking", true],
    ["blocked", false],
    ["error", false],
    ["warning", false],
    ["idle", false],
  ] as const)("reports the %s gate state", (status, loading) => {
    const gate = usePreflightGate(makePreflight({ status, loading }));
    expect(gate.reason).toBe(status === "checking" ? "checking" : status);
  });
});
