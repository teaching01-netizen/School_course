import { zoneLocalInputToUTCISO } from "@/utils/timezone";

export function yyyyMmDd(d: Date) {
  return d.toISOString().slice(0, 10);
}

export function formatTimeRange(startAt: string, endAt: string): string {
  try {
    const start = new Date(startAt);
    const end = new Date(endAt);
    const dateStr = start.toLocaleDateString("en-GB", { day: "numeric", month: "short" });
    const startTime = start.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
    const endTime = end.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
    return `${dateStr}, ${startTime}–${endTime}`;
  } catch {
    return `${startAt} → ${endAt}`;
  }
}

export function minutesBetween(startUTCISO: string, endUTCISO: string): number | null {
  const s = new Date(startUTCISO);
  const e = new Date(endUTCISO);
  if (Number.isNaN(s.getTime()) || Number.isNaN(e.getTime())) return null;
  return Math.round((e.getTime() - s.getTime()) / 60000);
}

export function fmtDuration(mins: number): string {
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  return `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
}

export function localDateTimeToUTCISO(local: string, zone: string): string | null {
  if (!local) return null;
  return zoneLocalInputToUTCISO(local, zone);
}
