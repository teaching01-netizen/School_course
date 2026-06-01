import type { ManagedAbsence } from "@/types";

function formatDate(value: string): string {
  return new Date(value + "T00:00:00").toLocaleDateString("en-GB", { day: "numeric", month: "short" });
}

function formatSessionDate(value: string): string {
  return new Date(value).toLocaleDateString("en-GB", { day: "numeric", month: "short" });
}

function missedSessionDates(absence: ManagedAbsence): string[] {
  const sessions = (absence.missed_sessions ?? [])
    .slice()
    .sort((left, right) => new Date(left.start_at).getTime() - new Date(right.start_at).getTime());

  const seen = new Set<string>();
  const out: string[] = [];
  for (const session of sessions) {
    const label = formatSessionDate(session.start_at);
    if (seen.has(label)) continue;
    seen.add(label);
    out.push(label);
  }
  return out;
}

export function formatAbsenceSummaryDates(absence: ManagedAbsence): string {
  const missedDates = missedSessionDates(absence);
  if (missedDates.length > 0) {
    return missedDates.join("\n");
  }
  const from = formatDate(absence.date_from);
  const to = formatDate(absence.date_to);
  return from === to ? from : `${from} - ${to}`;
}
