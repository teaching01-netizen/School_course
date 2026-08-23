import { DateTime } from "luxon";
import { formatUTCToZone } from "@/utils/timezone";

export type SessionDateFilter = Readonly<{
  from: string;
  to: string;
}>;

export type SessionDateWindow = Readonly<{
  start_at: string;
}>;

export const EMPTY_SESSION_DATE_FILTER: SessionDateFilter = {
  from: "",
  to: "",
};

function parseDateKey(value: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
  const date = DateTime.fromISO(value, { zone: "utc" });
  if (!date.isValid || date.toFormat("yyyy-MM-dd") !== value) return null;
  return value;
}

export function isSessionDateFilterActive(filter: SessionDateFilter): boolean {
  return filter.from !== "" || filter.to !== "";
}

export function validateSessionDateFilter(
  filter: SessionDateFilter,
): string | null {
  const from = filter.from === "" ? null : parseDateKey(filter.from);
  const to = filter.to === "" ? null : parseDateKey(filter.to);
  if (
    (filter.from !== "" && from === null) ||
    (filter.to !== "" && to === null)
  ) {
    return "Enter a valid calendar date.";
  }
  if (from !== null && to !== null && from > to) {
    return "From date must be earlier than or equal to To date.";
  }
  return null;
}

export function sessionMatchesDateFilter(
  session: SessionDateWindow,
  zone: string,
  filter: SessionDateFilter,
): boolean {
  if (!isSessionDateFilterActive(filter)) return true;
  if (validateSessionDateFilter(filter)) return false;

  const sessionDate = formatUTCToZone(session.start_at, zone, "yyyy-MM-dd");
  if (!sessionDate) return false;
  if (filter.from !== "" && sessionDate < filter.from) return false;
  if (filter.to !== "" && sessionDate > filter.to) return false;
  return true;
}
