import { describe, expect, it, vi, beforeEach, afterEach, beforeAll } from "vitest";
import {
  FeatureFlagProvider,
  useFeatureFlag,
  setFeatureFlag,
} from "../featureFlag";
import { render, screen } from "@testing-library/react";

/* ------------------------------------------------------------------ */
/*  localStorage mock for jsdom                                        */
/* ------------------------------------------------------------------ */

const store: Record<string, string> = {};

beforeAll(() => {
  Object.defineProperty(window, "localStorage", {
    value: {
      getItem: vi.fn((k: string) => store[k] ?? null),
      setItem: vi.fn((k: string, v: string) => { store[k] = String(v); }),
      removeItem: vi.fn((k: string) => { delete store[k]; }),
      clear: vi.fn(() => { for (const k in store) delete store[k]; }),
    },
    writable: true,
    configurable: true,
  });
});

/* ------------------------------------------------------------------ */
/*  useFeatureFlag hook                                                */
/* ------------------------------------------------------------------ */

function TestFlagConsumer({ name }: { name: string }) {
  const enabled = useFeatureFlag(name);
  return <span data-testid={`flag-${name}`}>{enabled ? "ON" : "OFF"}</span>;
}

describe("FeatureFlagProvider and useFeatureFlag", () => {
  beforeEach(() => {
    localStorage.clear();
    for (const k in store) delete store[k];
  });

  afterEach(() => {
    localStorage.clear();
    for (const k in store) delete store[k];
  });

  it("defaults all flags to off", () => {
    render(
      <FeatureFlagProvider>
        <TestFlagConsumer name="test_flag" />
      </FeatureFlagProvider>,
    );
    expect(screen.getByTestId("flag-test_flag")).toHaveTextContent("OFF");
  });

  it("reads initial flags from localStorage", () => {
    localStorage.setItem("schedule_impact_flags", JSON.stringify({ my_flag: true }));
    render(
      <FeatureFlagProvider>
        <TestFlagConsumer name="my_flag" />
      </FeatureFlagProvider>,
    );
    expect(screen.getByTestId("flag-my_flag")).toHaveTextContent("ON");
  });

  it("handles corrupt localStorage gracefully", () => {
    localStorage.setItem("schedule_impact_flags", "NOT_JSON");
    render(
      <FeatureFlagProvider>
        <TestFlagConsumer name="test" />
      </FeatureFlagProvider>,
    );
    expect(screen.getByTestId("flag-test")).toHaveTextContent("OFF");
  });
});

/* ------------------------------------------------------------------ */
/*  setFeatureFlag (standalone)                                        */
/* ------------------------------------------------------------------ */

describe("setFeatureFlag", () => {
  beforeEach(() => {
    localStorage.clear();
    for (const k in store) delete store[k];
  });

  afterEach(() => {
    localStorage.clear();
    for (const k in store) delete store[k];
  });

  it("writes a flag to localStorage", () => {
    setFeatureFlag("test_key", true);
    const stored = JSON.parse(localStorage.getItem("schedule_impact_flags") ?? "{}");
    expect(stored.test_key).toBe(true);
  });

  it("merges with existing flags", () => {
    localStorage.setItem("schedule_impact_flags", JSON.stringify({ existing: true }));
    setFeatureFlag("new_key", true);
    const stored = JSON.parse(localStorage.getItem("schedule_impact_flags") ?? "{}");
    expect(stored.existing).toBe(true);
    expect(stored.new_key).toBe(true);
  });

  it("can disable a flag", () => {
    setFeatureFlag("flag_to_disable", true);
    setFeatureFlag("flag_to_disable", false);
    const stored = JSON.parse(localStorage.getItem("schedule_impact_flags") ?? "{}");
    expect(stored.flag_to_disable).toBe(false);
  });
});
