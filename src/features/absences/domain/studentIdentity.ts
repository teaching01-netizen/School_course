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

/**
 * Echo mask for a just-typed nickname: first character + "***", mirroring
 * the server's nickname_hint rule. Single-character values reveal nothing
 * because the one character is the whole nickname.
 */
export function maskNickname(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  const chars = Array.from(trimmed);
  if (chars.length < 2) return "***";
  return `${chars[0]}***`;
}

export function getStudentDisplayName(lookup: StudentLookupResponse | null) {
  return lookup?.display_name?.trim() || lookup?.nickname?.trim() || lookup?.full_name?.trim() || "";
}
