import type { ResolutionAction, ResolutionActionPolicy } from "../types";

export type ResolvedAction = {
  action: ResolutionAction;
  allowed: boolean;
  reasonRequired: boolean;
  disabledReason: string | null;
  notificationExpected: boolean;
};

const AVAILABLE_ACTIONS: ResolutionAction[] = ["reassign", "keep", "cancel", "mark_for_review"];

/**
 * Resolve the presentation state of each action based on backend policy.
 * If no policy is provided, defaults to all actions being allowed.
 */
export function resolvePolicy(
  actionPolicy: ResolutionActionPolicy[] | undefined,
  availableActions: ResolutionAction[] = AVAILABLE_ACTIONS,
): ResolvedAction[] {
  const policy = (actionPolicy ?? []).filter(
    (a) => availableActions.includes(a.action),
  );

  if (policy.length === 0) {
    return availableActions.map((action) => ({
      action,
      allowed: true,
      reasonRequired: action === "mark_for_review",
      disabledReason: null,
      notificationExpected: action !== "mark_for_review",
    }));
  }

  return policy.map((p) => ({
    action: p.action,
    allowed: p.allowed,
    reasonRequired: p.reason_required,
    disabledReason: p.disabled_reason,
    notificationExpected: p.notification_expected,
  }));
}

/**
 * Get only the actions that are selectable (allowed and not disabled).
 */
export function getSelectableActions(
  actionPolicy: ResolutionActionPolicy[] | undefined,
): ResolutionAction[] {
  return resolvePolicy(actionPolicy)
    .filter((p) => p.allowed && !p.disabledReason)
    .map((p) => p.action);
}

/**
 * Get actions that are denied with their explanation.
 */
export function getDeniedActions(
  actionPolicy: ResolutionActionPolicy[] | undefined,
): Array<{ action: ResolutionAction; reason: string }> {
  return resolvePolicy(actionPolicy)
    .filter((p) => !p.allowed && p.disabledReason)
    .map((p) => ({ action: p.action, reason: p.disabledReason! }));
}
