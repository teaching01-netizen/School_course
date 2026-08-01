import { describe, expect, it } from "vitest";
import { scheduleImpactRoutes } from "../routes";

describe("scheduleImpactRoutes", () => {
  it("schedule points to /schedule", () => {
    expect(scheduleImpactRoutes.schedule).toBe("/schedule");
  });

  it("scheduleImpact points to /operations/schedule-impact", () => {
    expect(scheduleImpactRoutes.scheduleImpact).toBe("/operations/schedule-impact");
  });

  it("sessionChangeDetail generates correct path", () => {
    expect(scheduleImpactRoutes.sessionChangeDetail("change-42")).toBe(
      "/operations/session-changes/change-42",
    );
  });

  it("sessionChangeDetail handles different IDs", () => {
    expect(scheduleImpactRoutes.sessionChangeDetail("abc-123")).toBe(
      "/operations/session-changes/abc-123",
    );
  });

  it("absenceSettings points to /admin/absence-settings", () => {
    expect(scheduleImpactRoutes.absenceSettings).toBe("/admin/absence-settings");
  });

  it("notificationSettings includes section query param", () => {
    expect(scheduleImpactRoutes.notificationSettings).toBe(
      "/admin/absence-settings?section=notifications",
    );
  });

  it("static routes are non-empty strings", () => {
    expect(typeof scheduleImpactRoutes.schedule).toBe("string");
    expect(scheduleImpactRoutes.schedule.length).toBeGreaterThan(0);
    expect(typeof scheduleImpactRoutes.scheduleImpact).toBe("string");
    expect(scheduleImpactRoutes.scheduleImpact.length).toBeGreaterThan(0);
    expect(typeof scheduleImpactRoutes.absenceSettings).toBe("string");
    expect(scheduleImpactRoutes.absenceSettings.length).toBeGreaterThan(0);
    expect(typeof scheduleImpactRoutes.notificationSettings).toBe("string");
    expect(scheduleImpactRoutes.notificationSettings.length).toBeGreaterThan(0);
  });

  it("sessionChangeDetail is a function", () => {
    expect(typeof scheduleImpactRoutes.sessionChangeDetail).toBe("function");
  });
});
