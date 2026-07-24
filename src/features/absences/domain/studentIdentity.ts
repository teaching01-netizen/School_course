import type { StudentLookupResponse } from "../types";

export function normalizeLookupWcode(input: string): string {
  const trimmed = input.trim();
  if (!trimmed) return "";
  return trimmed[0]?.toLowerCase() === "w" ? `W${trimmed.slice(1)}` : trimmed;
}

export function isWCode(input: string): boolean {
  return /^W.+$/i.test(input.trim());
}

export function maskPhone(phone?: string | null): string {
  if (!phone) return "";
  const digits = phone.replace(/\D/g, "");
  if (digits.length <= 4) return phone;
  return `${digits.slice(0, 3)} *** ${digits.slice(-3)}`;
}

export function getStudentDisplayName(lookup: StudentLookupResponse | null) {
  return lookup?.display_name?.trim() || lookup?.nickname?.trim() || lookup?.full_name?.trim() || "";
}
