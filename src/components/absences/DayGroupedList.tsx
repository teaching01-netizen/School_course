import { useState, useMemo, useCallback } from "react";
import { motion, AnimatePresence, useReducedMotion } from "framer-motion";
import clsx from "clsx";

export type DayGroupedListSession = {
  id: string;
  date: string;
  start_at: string;
  end_at: string;
  subject_code: string;
};

type DayGroupedListProps = {
  sessions: DayGroupedListSession[];
  absentSessionIds: Set<string>;
  coverSessionIds: Set<string>;
  onToggleAbsent: (sessionId: string) => void;
  onToggleCover: (sessionId: string) => void;
};

type DayGroup = {
  dayName: string;
  sessions: DayGroupedListSession[];
};

const DAY_ORDER = [
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
  "Sunday",
] as const;

function getDayName(dateStr: string): string {
  const d = new Date(dateStr + "T12:00:00");
  return d.toLocaleDateString("en-GB", { weekday: "long" });
}

function formatTime(iso: string): string {
  const timePart = iso.split("T")[1];
  if (!timePart) return "";
  const [h, m] = timePart.split(":");
  return `${h}:${m}`;
}

function groupByDay(sessions: DayGroupedListSession[]): DayGroup[] {
  const map = new Map<string, DayGroupedListSession[]>();
  for (const s of sessions) {
    const day = getDayName(s.date);
    const existing = map.get(day);
    if (existing) {
      existing.push(s);
    } else {
      map.set(day, [s]);
    }
  }

  const groups: DayGroup[] = [];
  for (const dayName of DAY_ORDER) {
    const daySessions = map.get(dayName);
    if (daySessions) {
      groups.push({ dayName, sessions: daySessions });
    }
  }
  return groups;
}

export default function DayGroupedList({
  sessions,
  absentSessionIds,
  coverSessionIds,
  onToggleAbsent,
  onToggleCover,
}: DayGroupedListProps) {
  const reduceMotion = useReducedMotion();
  const groups = useMemo(() => groupByDay(sessions), [sessions]);

  // All expanded by default
  const [expandedDays, setExpandedDays] = useState<Set<string>>(() => {
    return new Set(groups.map((g) => g.dayName));
  });

  const toggleDay = useCallback((dayName: string) => {
    setExpandedDays((prev) => {
      const next = new Set(prev);
      if (next.has(dayName)) {
        next.delete(dayName);
      } else {
        next.add(dayName);
      }
      return next;
    });
  }, []);

  const handleChipClick = useCallback(
    (id: string) => {
      onToggleAbsent(id);
    },
    [onToggleAbsent],
  );

  const handleChipDoubleClick = useCallback(
    (id: string) => {
      onToggleCover(id);
    },
    [onToggleCover],
  );

  const handleChipKeyDown = useCallback(
    (e: React.KeyboardEvent, id: string) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        onToggleAbsent(id);
      }
    },
    [onToggleAbsent],
  );

  if (sessions.length === 0) {
    return (
      <div className="rounded-sm border border-gray-200 bg-white p-5 text-center text-gray-650 font-medium">
        No sessions found in this date range
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {groups.map((group) => {
        const isExpanded = expandedDays.has(group.dayName);

        return (
          <div
            key={group.dayName}
            className="rounded-sm border border-gray-200 bg-white overflow-hidden"
          >
            {/* Day header — collapsible */}
            <button
              type="button"
              aria-expanded={isExpanded}
              aria-label={`${group.dayName}, ${group.sessions.length} session${group.sessions.length === 1 ? "" : "s"}`}
              onClick={() => toggleDay(group.dayName)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  toggleDay(group.dayName);
                }
              }}
              className={clsx(
                "flex w-full items-center justify-between px-4 py-3 text-left text-sm font-semibold transition-colors",
                "bg-gray-50 hover:bg-gray-100 focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-green-500",
              )}
            >
              <span className="text-gray-800">
                {group.dayName}
              </span>
              <span className="flex items-center gap-2">
                <span className="rounded-full bg-gray-200 px-2 py-0.5 text-xs font-medium text-gray-600">
                  {group.sessions.length} session{group.sessions.length === 1 ? "" : "s"}
                </span>
                <motion.span
                  animate={{ rotate: isExpanded ? 0 : -90 }}
                  transition={{ duration: reduceMotion ? 0 : 0.15 }}
                  aria-hidden="true"
                  className="text-gray-500"
                >
                  ▼
                </motion.span>
              </span>
            </button>

            {/* Session list — animated expand/collapse */}
            <AnimatePresence initial={false}>
              {isExpanded && (
                <motion.div
                  key="content"
                  initial={reduceMotion ? false : { height: 0, opacity: 0 }}
                  animate={{ height: "auto", opacity: 1 }}
                  exit={reduceMotion ? undefined : { height: 0, opacity: 0 }}
                  transition={{ duration: reduceMotion ? 0 : 0.2 }}
                  className="overflow-hidden"
                >
                  <div className="flex flex-col gap-1.5 p-2">
                    {group.sessions.map((session) => {
                      const isAbsent = absentSessionIds.has(session.id);
                      const isCover = coverSessionIds.has(session.id);
                      const isActive = isAbsent || isCover;

                      return (
                        <button
                          key={session.id}
                          type="button"
                          role="button"
                          aria-pressed={isActive}
                          aria-label={`${formatTime(session.start_at)}\u2013${formatTime(session.end_at)} ${session.subject_code}${isAbsent ? " absent" : ""}${isCover ? " cover" : ""}`}
                          onClick={() => handleChipClick(session.id)}
                          onDoubleClick={() => handleChipDoubleClick(session.id)}
                          onKeyDown={(e) => handleChipKeyDown(e, session.id)}
                          className={clsx(
                            "flex w-full items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium transition-colors min-h-[44px]",
                            isAbsent &&
                              "border-l-4 border-l-red-500 bg-red-50 border-red-200 text-red-800",
                            isCover &&
                              !isAbsent &&
                              "border-l-4 border-l-amber-500 bg-amber-50 border-amber-200 text-amber-800",
                            !isActive &&
                              "border-l-4 border-l-gray-300 bg-gray-50 border-gray-200 text-gray-600",
                          )}
                        >
                          {isAbsent && (
                            <span aria-hidden="true" className="text-xs">⊘</span>
                          )}
                          {isCover && !isAbsent && (
                            <span aria-hidden="true" className="text-xs">↻</span>
                          )}
                          <span>
                            {formatTime(session.start_at)}–{formatTime(session.end_at)}
                          </span>
                          <span className="ml-auto text-xs text-gray-500">
                            {session.subject_code}
                          </span>
                        </button>
                      );
                    })}
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        );
      })}
    </div>
  );
}
