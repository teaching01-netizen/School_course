const messageByReasonCode: Record<string, string> = {
  before_request_date: "This make-up time has passed",
  outside_cutoff: "This date is outside the make-up period",
  target_final_class: "The final class can't be used as a make-up",
  overlaps_missed_class: "This make-up class overlaps your absence",
  sit_in_session_already_used: "You've already used this make-up class",
  same_occurrence_missing: "No matching lesson is available",
};

export function getAbsenceMessageCopy(reasonCode?: string | null): string {
  const message = reasonCode && Object.prototype.hasOwnProperty.call(messageByReasonCode, reasonCode)
    ? messageByReasonCode[reasonCode]
    : undefined;
  return message ?? "This make-up class isn’t available.";
}
