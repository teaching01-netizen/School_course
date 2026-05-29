import { DateTime } from "luxon";

export function formatUTCToZone(utcISO: string, zone: string, fmt: string): string | null {
  const dt = DateTime.fromISO(utcISO, { zone: "utc" });
  if (!dt.isValid) return null;
  const z = dt.setZone(zone);
  if (!z.isValid) return null;
  return z.toFormat(fmt);
}

export function zoneDateToUTCISO(dateYYYYMMDD: string, zone: string, endOfDay = false): string | null {
  const dt = DateTime.fromISO(dateYYYYMMDD, { zone });
  if (!dt.isValid) return null;
  const d = endOfDay ? dt.endOf("day") : dt.startOf("day");
  return d.toUTC().toISO({ suppressMilliseconds: false });
}

export function zoneLocalInputToUTCISO(localYYYYMMDDTHHMM: string, zone: string): string | null {
  // <input type="datetime-local"> => "YYYY-MM-DDTHH:mm"
  const dt = DateTime.fromISO(localYYYYMMDDTHHMM, { zone });
  if (!dt.isValid) return null;
  return dt.toUTC().toISO({ suppressMilliseconds: false });
}

export function utcISOToZoneLocalInput(utcISO: string, zone: string): string | null {
  const dt = DateTime.fromISO(utcISO, { zone: "utc" });
  if (!dt.isValid) return null;
  const z = dt.setZone(zone);
  if (!z.isValid) return null;
  return z.toFormat("yyyy-MM-dd'T'HH:mm");
}

export function utcISOToZoneDate(utcISO: string, zone: string): string | null {
  const dt = DateTime.fromISO(utcISO, { zone: "utc" });
  if (!dt.isValid) return null;
  const z = dt.setZone(zone);
  if (!z.isValid) return null;
  return z.toFormat("yyyy-MM-dd");
}

