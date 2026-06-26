import type { UsePreflightReturn } from "./usePreflight";

type PreflightGateStatus = "idle" | "available" | "provisional" | "blocked";

interface UsePreflightGateOptions {
  requiredFields?: string[];
  isFormValid?: boolean;
}

interface UsePreflightGateReturn {
  canSave: boolean;
  status: PreflightGateStatus;
  isChecking: boolean;
  reason: "idle" | "checking" | "blocked" | "ok" | "no_fields";
}

export default function usePreflightGate(
  preflight: UsePreflightReturn,
  options: UsePreflightGateOptions = {}
): UsePreflightGateReturn {
  const { requiredFields = [], isFormValid = true } = options;
  const status = preflight.status as PreflightGateStatus;
  const isChecking = preflight.loading;

  const fieldsFilled = requiredFields.length === 0 || requiredFields.every(Boolean);

  const canSave = (status === "available" || status === "provisional") && !isChecking && isFormValid;

  let reason: UsePreflightGateReturn["reason"] = "ok";
  if (isChecking) reason = "checking";
  else if (status === "idle") reason = fieldsFilled ? "idle" : "no_fields";
  else if (status === "blocked") reason = "blocked";
  else if (!isFormValid) reason = "idle";

  return { canSave, status, isChecking, reason };
}
