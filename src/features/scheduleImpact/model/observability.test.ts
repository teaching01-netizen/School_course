import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { emitImpactEvent, IMPACT_EVENTS } from "../observability";

describe("IMPACT_EVENTS", () => {
  it("contains all expected event names", () => {
    expect(IMPACT_EVENTS.QUEUE_LOADED).toBe("schedule_impact.queue_loaded");
    expect(IMPACT_EVENTS.ISSUE_OPENED).toBe("schedule_impact.issue_opened");
    expect(IMPACT_EVENTS.PREVIEW_GENERATED).toBe("schedule_impact.preview_generated");
    expect(IMPACT_EVENTS.RESOLUTION_SUCCEEDED).toBe("schedule_impact.resolution_succeeded");
    expect(IMPACT_EVENTS.RESOLUTION_CONFLICT).toBe("schedule_impact.resolution_conflict");
    expect(IMPACT_EVENTS.RESOLUTION_FAILED).toBe("schedule_impact.resolution_failed");
    expect(IMPACT_EVENTS.ANALYSIS_RETRIED).toBe("schedule_impact.analysis_retried");
    expect(IMPACT_EVENTS.NOTIFICATION_FAILED).toBe("schedule_impact.notification_failed");
  });

  it("all event names start with schedule_impact.", () => {
    for (const value of Object.values(IMPACT_EVENTS)) {
      expect(value).toMatch(/^schedule_impact\./);
    }
  });
});

describe("emitImpactEvent", () => {
  let dispatchedEvents: CustomEvent[] = [];
  let dispatchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    dispatchedEvents = [];
    dispatchSpy = vi.spyOn(window, "dispatchEvent").mockImplementation((event) => {
      if (event instanceof CustomEvent) {
        dispatchedEvents.push(event);
      }
      return true;
    });
  });

  afterEach(() => {
    dispatchSpy.mockRestore();
  });

  it("dispatches a CustomEvent with the event name", () => {
    emitImpactEvent("test.event");
    expect(dispatchedEvents).toHaveLength(1);
    expect(dispatchedEvents[0].type).toBe("schedule-impact");
    expect(dispatchedEvents[0].detail).toEqual({ event: "test.event" });
  });

  it("includes data in the event detail", () => {
    emitImpactEvent("test.event", { issue_id: "abc", severity: "critical" });
    expect(dispatchedEvents[0].detail).toEqual({
      event: "test.event",
      issue_id: "abc",
      severity: "critical",
    });
  });

  it("omits data key when not provided", () => {
    emitImpactEvent("test.event");
    expect(dispatchedEvents[0].detail).toEqual({ event: "test.event" });
  });

  it("dispatches real IMPACT_EVENTS constants", () => {
    emitImpactEvent(IMPACT_EVENTS.ISSUE_OPENED, { issue_id: "1" });
    expect(dispatchedEvents[0].detail.event).toBe("schedule_impact.issue_opened");
  });
});
