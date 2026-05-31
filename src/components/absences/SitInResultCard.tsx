import { useMemo } from "react";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

export type SessionBrief = { id: string; start_at: string; end_at: string };

export type SitInResult = {
  sit_in_method: "physical" | "zoom" | "pending";
  sit_in_course?: { id: string; code: string; name: string };
  missed_count: number;
  missed_sessions?: SessionBrief[];
  available_sessions?: SessionBrief[];
  pre_selected?: SessionBrief[];
};

export type SitInResultCardProps = {
  subjectCode: string;
  subjectName: string;
  result: SitInResult;
  selectedSessionIds: Set<string>;
  onToggleSession: (sessionId: string) => void;
  maxSessions?: number;
  zoomDescription?: string;
  error?: string;
  onRetry?: () => void;
  onSkip?: () => void;
};

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function fmtTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function fmtDay(iso: string): string {
  return new Date(iso.slice(0, 10) + "T00:00:00").toLocaleDateString(
    "en-GB",
    { weekday: "short", day: "numeric", month: "short" },
  );
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

export default function SitInResultCard({
  subjectCode,
  subjectName,
  result,
  selectedSessionIds,
  onToggleSession,
  zoomDescription,
  error,
  onRetry,
  onSkip,
}: SitInResultCardProps) {
  /* ---------- error overlay ---------- */
  if (error) {
    return (
      <div className="rounded-sm border border-red-200 bg-red-50 p-3 text-sm text-red-800">
        <p className="font-medium">{error}</p>
        <div className="mt-2 flex gap-2">
          {onRetry && (
              <button
                type="button"
                className="min-h-[44px] rounded-sm border border-red-300 bg-white px-3 py-2 text-xs font-medium text-red-700 transition-colors hover:bg-red-100"
                onClick={onRetry}
              >
                Retry
              </button>
          )}
          {onSkip && (
              <button
                type="button"
                className="min-h-[44px] rounded-sm border border-gray-300 bg-white px-3 py-2 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-100"
                onClick={onSkip}
              >
                Skip
              </button>
          )}
        </div>
      </div>
    );
  }

  /* ---------- zoom ---------- */
  if (result.sit_in_method === "zoom") {
    return (
      <div className="rounded-sm border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800">
        {zoomDescription ?? "Zoom session - no physical class attendance required."}
        {result.missed_count > 0 && (
          <p className="mt-1 text-xs text-blue-600">
            You will miss {result.missed_count} session(s).
          </p>
        )}
      </div>
    );
  }

  /* ---------- pending ---------- */
  if (result.sit_in_method === "pending") {
    return (
      <div className="rounded-sm border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
        Your sit-in plan will be assigned by staff after review.
      </div>
    );
  }

  /* ---------- physical ---------- */
  const missed = result.missed_sessions ?? [];
  const available = result.available_sessions ?? [];

  if (missed.length === 0 && available.length === 0) {
    return (
      <div className="rounded-sm border border-green-200 bg-green-50 p-3 text-sm text-green-800">
        Sit-in at <strong>{result.sit_in_course?.code ?? subjectCode}</strong>{" "}
        &mdash; {result.sit_in_course?.name ?? subjectName}
        <p className="mt-1 text-xs text-green-600">
          {result.missed_count} missed session(s).
        </p>
        <p className="mt-2 text-sm text-gray-500">
          No sessions in this date range.
        </p>
      </div>
    );
  }

  /* group available sessions by date */
  const availByDate = new Map<string, SessionBrief[]>();
  for (const a of available) {
    const date = a.start_at.slice(0, 10);
    if (!availByDate.has(date)) availByDate.set(date, []);
    availByDate.get(date)!.push(a);
  }

  /* collect all dates and sort */
  const allDates = new Set<string>();
  for (const m of missed) allDates.add(m.start_at.slice(0, 10));
  for (const a of available) allDates.add(a.start_at.slice(0, 10));
  const sortedDates = [...allDates].sort();

  /* pair missed with available (greedy, one-to-one per date) — computed before render */
  type PairedDay = { date: string; dayMissed: Array<{ m: SessionBrief; pair: SessionBrief | null }> };
  const pairedDays: PairedDay[] = useMemo(() => {
    const usedAvail = new Set<string>();
    return sortedDates.map((date) => {
      const dayMissed = missed.filter((m) => m.start_at.slice(0, 10) === date);
      const dayAvail = availByDate.get(date) ?? [];
      const pairs = dayMissed.map((m) => {
        const pair = dayAvail.find((a) => !usedAvail.has(a.id)) ?? null;
        if (pair) usedAvail.add(pair.id);
        return { m, pair };
      });
      return { date, dayMissed: pairs };
    });
  }, [missed, available, sortedDates.join(",")]);

  return (
    <div className="space-y-3">
      {/* green banner */}
      <div className="rounded-sm border border-green-200 bg-green-50 p-3 text-sm text-green-800">
        Sit-in at{" "}
        <strong>{result.sit_in_course?.code ?? subjectCode}</strong> &mdash;{" "}
        {result.sit_in_course?.name ?? subjectName}
        <p className="mt-1 text-xs text-green-600">
          {result.missed_count} missed session(s).
          {available.length > 0
            ? ` ${available.length} sit-in session(s) available.`
            : ""}
        </p>
      </div>

      {/* day-by-day timeline */}
      {pairedDays.map(({ date, dayMissed }) => (
        <div key={date}>
          <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
            {fmtDay(date)}
          </p>
          <div className="space-y-1">
            {dayMissed.map(({ m, pair }) => (
              <div
                key={m.id}
                className="max-sm:flex-col max-sm:items-start flex items-center gap-2 rounded-sm px-2 py-1 hover:bg-gray-50"
              >
                <div className="flex-1 text-sm text-gray-700">
                  Missed: {fmtTime(m.start_at)} &ndash; {fmtTime(m.end_at)}
                </div>
                {pair ? (
                  <label className="sm:ml-auto flex cursor-pointer shrink-0 items-center gap-1.5 min-h-[44px] text-sm">
                    <input
                      type="checkbox"
                      checked={selectedSessionIds.has(pair.id)}
                      onChange={() => onToggleSession(pair.id)}
                      className="accent-[var(--color-wi-green)] h-5 w-5"
                    />
                    <span className="text-green-700">
                      Sit-in: {fmtTime(pair.start_at)} &ndash;{" "}
                      {fmtTime(pair.end_at)}
                    </span>
                  </label>
                ) : (
                  <span className="sm:ml-auto shrink-0 text-xs italic text-gray-400">
                    &mdash; (full)
                  </span>
                )}
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
