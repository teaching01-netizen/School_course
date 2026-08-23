import { formatUTCToZone } from "@/utils/timezone";

export type SessionTimeFilter = Readonly<{
  from: string;
  to: string;
}>;

export type SessionTimeWindow = Readonly<{
  start_at: string;
  end_at: string;
}>;

export const EMPTY_SESSION_TIME_FILTER: SessionTimeFilter = {
  from: "",
  to: "",
};

function parseTimeToMinutes(value: string): number | null {
  if (!/^\d{2}:\d{2}$/.test(value)) return null;
  const [hoursText, minutesText] = value.split(":");
  const hours = Number(hoursText);
  const minutes = Number(minutesText);
  if (
    !Number.isInteger(hours) ||
    !Number.isInteger(minutes) ||
    hours > 23 ||
    minutes > 59
  )
    return null;
  return hours * 60 + minutes;
}

export function isSessionTimeFilterActive(filter: SessionTimeFilter): boolean {
  return filter.from !== "" || filter.to !== "";
}

export function validateSessionTimeFilter(
  filter: SessionTimeFilter,
): string | null {
  const from = filter.from === "" ? null : parseTimeToMinutes(filter.from);
  const to = filter.to === "" ? null : parseTimeToMinutes(filter.to);
  if (
    (filter.from !== "" && from === null) ||
    (filter.to !== "" && to === null)
  ) {
    return "Enter a valid time in 24-hour format.";
  }
  if (from !== null && to !== null && from > to) {
    return "From time must be earlier than or equal to To time.";
  }
  return null;
}

export function sessionMatchesTimeFilter(
  session: SessionTimeWindow,
  zone: string,
  filter: SessionTimeFilter,
): boolean {
  if (!isSessionTimeFilterActive(filter)) return true;
  if (validateSessionTimeFilter(filter)) return false;

  const startLabel = formatUTCToZone(session.start_at, zone, "HH:mm");
  const endLabel = formatUTCToZone(session.end_at, zone, "HH:mm");
  if (!startLabel || !endLabel) return false;

  const start = parseTimeToMinutes(startLabel);
  const end = parseTimeToMinutes(endLabel);
  const from = filter.from === "" ? null : parseTimeToMinutes(filter.from);
  const to = filter.to === "" ? null : parseTimeToMinutes(filter.to);
  if (start === null || end === null) return false;
  if (from !== null && start < from) return false;
  if (to !== null && end > to) return false;
  return true;
}
