import type { UsePreflightReturn } from "./usePreflight";

type PreflightGateReason =
  | "idle"
  | "checking"
  | "blocked"
  | "error"
  | "ok"
  | "no_fields";

interface UsePreflightGateOptions {
  requiredFields?: string[];
  isFormValid?: boolean;
}

interface UsePreflightGateReturn {
  canSave: boolean;
  status: UsePreflightReturn["status"];
  isChecking: boolean;
  reason: PreflightGateReason;
}

export default function usePreflightGate(
  preflight: UsePreflightReturn,
  options: UsePreflightGateOptions = {}
): UsePreflightGateReturn {
  const { requiredFields = [], isFormValid = true } = options;
  const status = preflight.status;
  const isChecking = preflight.loading;

  const fieldsFilled = requiredFields.length === 0 || requiredFields.every(Boolean);
  const canSave = fieldsFilled && (status === "available" || status === "provisional") && !isChecking && isFormValid;

  let reason: PreflightGateReason = "ok";
  if (isChecking) reason = "checking";
  else if (!fieldsFilled) reason = "no_fields";
  else if (status === "blocked") reason = "blocked";
  else if (status === "error") reason = "error";
  else if (status === "idle") reason = "idle";
  else if (!isFormValid) reason = "idle";

  return { canSave, status, isChecking, reason };
}
