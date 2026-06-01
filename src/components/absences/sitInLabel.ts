import type { ManagedAbsence } from "@/types";

export function formatSitInLabel(absence: ManagedAbsence): string {
  if (absence.sit_in_method === "zoom") {
    return "Zoom";
  }

  return absence.sit_in_subject_name
    ?? absence.sit_in_course_name
    ?? absence.sit_in_course_code
    ?? "Physical";
}
