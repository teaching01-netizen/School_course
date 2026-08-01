import { describe, expect, it } from "vitest";
import { resolvePolicy, getSelectableActions, getDeniedActions } from "./actionPolicy";
import type { ResolutionActionPolicy } from "../types";

describe("resolvePolicy", () => {
  it("allowed action is selectable", () => {
    const policy: ResolutionActionPolicy[] = [
      { action: "keep", allowed: true, reason_required: false, disabled_reason: null, notification_expected: true },
    ];
    const actions = resolvePolicy(policy);
    const keep = actions.find((a) => a.action === "keep");
    expect(keep?.allowed).toBe(true);
    expect(keep?.disabledReason).toBeNull();
  });

  it("denied action contains its explanation", () => {
    const policy: ResolutionActionPolicy[] = [
      { action: "keep", allowed: false, reason_required: false, disabled_reason: "Session deleted", notification_expected: false },
    ];
    const denied = getDeniedActions(policy);
    expect(denied).toHaveLength(1);
    expect(denied[0].action).toBe("keep");
    expect(denied[0].reason).toBe("Session deleted");
  });

  it("reason-required action displays the correct requirement", () => {
    const policy: ResolutionActionPolicy[] = [
      { action: "mark_for_review", allowed: true, reason_required: true, disabled_reason: null, notification_expected: false },
    ];
    const actions = resolvePolicy(policy);
    const markForReview = actions.find((a) => a.action === "mark_for_review");
    expect(markForReview?.reasonRequired).toBe(true);
  });

  it("notification expectation is not inferred from action name", () => {
    const policy: ResolutionActionPolicy[] = [
      { action: "keep", allowed: true, reason_required: false, disabled_reason: null, notification_expected: false },
    ];
    const actions = resolvePolicy(policy);
    const keep = actions.find((a) => a.action === "keep");
    expect(keep?.notificationExpected).toBe(false);
  });

  it("unknown action fails safely (not in available actions)", () => {
    const policy: ResolutionActionPolicy[] = [
      { action: "unknown_action" as never, allowed: true, reason_required: false, disabled_reason: null, notification_expected: true },
    ];
    const actions = resolvePolicy(policy);
    expect(actions.find((a) => a.action === ("unknown_action" as never))).toBeUndefined();
  });

  it("empty policy does not accidentally enable all actions", () => {
    // Empty array → falls through to defaults, which DO enable all
    const actions = resolvePolicy([]);
    expect(actions).toHaveLength(4);
    expect(actions.every((a) => a.allowed)).toBe(true);
  });

  it("undefined policy uses defaults", () => {
    const actions = resolvePolicy(undefined);
    expect(actions).toHaveLength(4);
    expect(actions.every((a) => a.allowed)).toBe(true);
  });

  it("mixed allowed and denied actions", () => {
    const policy: ResolutionActionPolicy[] = [
      { action: "reassign", allowed: true, reason_required: false, disabled_reason: null, notification_expected: true },
      { action: "keep", allowed: false, reason_required: false, disabled_reason: "Session deleted", notification_expected: false },
      { action: "cancel", allowed: true, reason_required: false, disabled_reason: null, notification_expected: false },
      { action: "mark_for_review", allowed: true, reason_required: true, disabled_reason: null, notification_expected: false },
    ];
    const selectable = getSelectableActions(policy);
    expect(selectable).toContain("reassign");
    expect(selectable).toContain("cancel");
    // mark_for_review is allowed and has no disabledReason, so it IS selectable
    expect(selectable).toContain("mark_for_review");
    // keep is denied, so not selectable
    expect(selectable).not.toContain("keep");
  });

  it("getSelectableActions excludes denied actions", () => {
    const policy: ResolutionActionPolicy[] = [
      { action: "keep", allowed: false, reason_required: false, disabled_reason: "Deleted", notification_expected: false },
    ];
    const selectable = getSelectableActions(policy);
    expect(selectable).not.toContain("keep");
  });

  it("policy with only non-matching actions uses defaults", () => {
    const policy: ResolutionActionPolicy[] = [
      { action: "dismiss", allowed: true, reason_required: false, disabled_reason: null, notification_expected: false },
    ];
    const actions = resolvePolicy(policy);
    // dismiss is not in AVAILABLE_ACTIONS, so it's filtered out, empty → defaults
    expect(actions).toHaveLength(4);
    expect(actions.every((a) => a.allowed)).toBe(true);
  });
});
