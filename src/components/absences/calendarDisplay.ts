import type { AbsenceStatus, CalendarAbsence, CalendarSessionBrief, CalendarSitInStudent } from "../../types";

export function formatFullDayLabel(dayKey: string): string {
  return new Date(`${dayKey}T00:00:00`).toLocaleDateString("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}

export function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
}

export function titleCase(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1).replace(/_/g, " ");
}

export function formatCount(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

export function getSessionLabel(session: CalendarSessionBrief): string {
  return session.subject_name?.trim() || session.course_name?.trim() || session.course_code?.trim() || "Session";
}

export function getAbsenceStudentLabel(absence: CalendarAbsence): string {
  const name = absence.student_name?.trim();
  return name ? `${absence.wcode} · ${name}` : absence.wcode;
}

export function getAbsenceSubjectLabel(absence: CalendarAbsence): string {
  return absence.subject_name?.trim() || absence.subject_code?.trim() || "Subject";
}

export function getSitInLabel(absence: CalendarAbsence): string {
  switch (absence.sit_in_method) {
    case "zoom":
      return "Zoom";
    case "physical":
      return absence.sit_in_subject_name?.trim() || absence.sit_in_course_name?.trim() || "To arrange";
    case "teacher_case":
      return "To arrange";
    default:
      return "To arrange";
  }
}

export function getSitInVisitorLabel(student: CalendarSitInStudent): string {
  const name = student.nickname?.trim() || student.student_name?.trim();
  const course = student.from_course_name?.trim() || student.from_course_code;
  return name ? `${name} (${student.wcode}) — ${course}` : `${student.wcode} — ${course}`;
}

export function absenceInlineClasses(absence: Pick<CalendarAbsence, "sit_in_method">): string {
  switch (absence.sit_in_method) {
    case "physical":
      return "border-amber-200 bg-amber-50/70";
    case "zoom":
      return "border-sky-200 bg-sky-50/70";
    default:
      return "border-rose-200 bg-rose-50/70";
  }
}

export function statusBadgeClasses(status: AbsenceStatus): string {
  switch (status) {
    case "pending":
      return "bg-blue-50 text-blue-700 border-blue-200";
    case "reviewed":
      return "bg-emerald-50 text-emerald-700 border-emerald-200";
    case "actioned":
      return "bg-slate-100 text-slate-600 border-slate-200";
    case "cancelled":
      return "bg-red-50 text-red-700 border-red-200";
    default:
      return "bg-gray-100 text-gray-600 border-gray-200";
  }
}
