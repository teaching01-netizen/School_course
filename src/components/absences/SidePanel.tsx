import { useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { X } from "lucide-react";
import type { CalendarAbsence, CalendarSessionBrief } from "../../types";
import EmptyState from "../ui/EmptyState";
import SidePanelAbsenceRow from "./SidePanelAbsenceRow";
import SidePanelSitInCard from "./SidePanelSitInCard";
import SidePanelStudentDetail from "./SidePanelStudentDetail";
import {
  formatCount,
  formatFullDayLabel,
  formatTime,
  getSitInVisitorLabel,
  getSessionLabel,
} from "./calendarDisplay";

export type AbsencePanelTab = "sit-ins" | "absences";

type SidePanelProps = {
  dayKey: string;
  sessions: CalendarSessionBrief[];
  absences: CalendarAbsence[];
  initialTab: AbsencePanelTab;
  onClose: () => void;
};

export default function SidePanel({ dayKey, sessions, absences, initialTab, onClose }: SidePanelProps) {
  const reduceMotion = useReducedMotion();
  const panelRef = useRef<HTMLDivElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);
  const [activeTab, setActiveTab] = useState<AbsencePanelTab>(initialTab);
  const [studentAbsence, setStudentAbsence] = useState<CalendarAbsence | null>(null);

  useEffect(() => {
    setActiveTab(initialTab);
    setStudentAbsence(null);
  }, [dayKey, initialTab]);

  useEffect(() => {
    previousFocus.current = document.activeElement as HTMLElement;
    document.body.style.overflow = "hidden";
    const panel = panelRef.current;
    const firstFocusable = panel?.querySelector<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
    firstFocusable?.focus();

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.body.style.overflow = "";
      previousFocus.current?.focus();
    };
  }, [onClose]);

  function handleTrapFocus(event: React.KeyboardEvent) {
    if (event.key !== "Tab") return;
    const panel = panelRef.current;
    if (!panel) return;
    const focusable = panel.querySelectorAll<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  const dayLabel = useMemo(() => formatFullDayLabel(dayKey), [dayKey]);
  const sitInAbsences = useMemo(() => {
    const assignedIds = new Set(sessions.flatMap((session) => (session.sit_in_students ?? []).map((student) => student.absence_id)));
    const explicitSitIns = absences.filter((absence) => absence.sit_in_method === "physical" || absence.sit_in_method === "zoom");
    return absences.filter((absence) => assignedIds.has(absence.id) || explicitSitIns.includes(absence));
  }, [absences, sessions]);
  const title = `${dayLabel} · ${formatCount(sessions.length, "session")} · ${formatCount(absences.length, "absence")}`;

  return (
    <AnimatePresence>
      <motion.div
        key="absence-panel-backdrop"
        className="fixed inset-0 z-50 bg-black/20"
        aria-hidden="true"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        transition={{ duration: reduceMotion ? 0 : 0.15 }}
        onClick={onClose}
      />
      <motion.aside
        key="absence-panel"
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="absence-panel-title"
        className="fixed inset-y-0 right-0 z-50 flex w-full flex-col bg-white shadow-sm outline-none min-[900px]:w-[420px]"
        initial={{ x: reduceMotion ? 0 : "100%" }}
        animate={{ x: 0 }}
        exit={{ x: reduceMotion ? 0 : "100%" }}
        transition={{ duration: reduceMotion ? 0 : 0.2, ease: "easeOut" }}
        onClick={(event) => event.stopPropagation()}
        onKeyDown={handleTrapFocus}
      >
        <header className="sticky top-0 z-10 border-b border-gray-200 bg-white">
          <div className="flex items-start justify-between gap-3 px-4 py-3">
            <div className="min-w-0">
              <h2 id="absence-panel-title" className="text-base font-semibold text-[var(--color-wi-text)]">{title}</h2>
              <span className="sr-only">Sessions ({sessions.length})</span>
              <p className="mt-0.5 text-xs text-gray-500">
                {sitInAbsences.length} sit-ins · {absences.length} absences
              </p>
            </div>
            <button type="button" className="rounded-sm p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700" aria-label="Close dialog" onClick={onClose}>
              <X className="h-5 w-5" aria-hidden="true" />
            </button>
          </div>
          {!studentAbsence ? (
            <div className="flex border-t border-gray-100 px-4">
              <button
                type="button"
                className={`min-h-[44px] border-b-2 px-3 text-sm font-semibold ${activeTab === "sit-ins" ? "border-[var(--color-wi-primary)] text-gray-900" : "border-transparent text-gray-500 hover:text-gray-900"}`}
                onClick={() => setActiveTab("sit-ins")}
              >
                Sit-ins ({sitInAbsences.length})
              </button>
              <button
                type="button"
                className={`min-h-[44px] border-b-2 px-3 text-sm font-semibold ${activeTab === "absences" ? "border-[var(--color-wi-primary)] text-gray-900" : "border-transparent text-gray-500 hover:text-gray-900"}`}
                onClick={() => setActiveTab("absences")}
              >
                Absences ({absences.length})
              </button>
            </div>
          ) : null}
        </header>

        <div className="flex-1 overflow-y-auto p-4">
          <AnimatePresence mode="wait" initial={false}>
            {studentAbsence ? (
              <motion.div
                key="student"
                initial={{ x: reduceMotion ? 0 : 24, opacity: 0 }}
                animate={{ x: 0, opacity: 1 }}
                exit={{ x: reduceMotion ? 0 : 24, opacity: 0 }}
                transition={{ duration: reduceMotion ? 0 : 0.18 }}
              >
                <SidePanelStudentDetail
                  absence={studentAbsence}
                  absences={absences}
                  dayLabel={dayLabel}
                  onBack={() => setStudentAbsence(null)}
                />
              </motion.div>
            ) : activeTab === "sit-ins" ? (
              <motion.div key="sit-ins" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: reduceMotion ? 0 : 0.1 }}>
                <section className="space-y-2">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">Day sessions ({sessions.length})</h3>
                  {sessions.length === 0 ? (
                    <p className="text-sm text-gray-400">No sessions this day.</p>
                  ) : (
                    sessions.map((session) => (
                      <article key={session.id} className="rounded-sm border border-gray-100 bg-gray-50 p-3 text-sm">
                        <p className="font-medium text-gray-800">{getSessionLabel(session)}</p>
                        <p className="text-xs text-gray-500">
                          {formatTime(session.start_at)} - {formatTime(session.end_at)}
                          {session.room_name ? ` · ${session.room_name}` : ""}
                        </p>
                        {session.sit_in_students?.length ? (
                          <p className="mt-2 border-t border-gray-100 pt-2 text-xs text-amber-700">
                            <span className="font-semibold">Visitors:</span>{" "}
                            {session.sit_in_students.map((student, index) => (
                              <span key={`${student.wcode}-${student.absence_id}`}>
                                {index > 0 ? ", " : ""}
                                {getSitInVisitorLabel(student)}
                              </span>
                            ))}
                          </p>
                        ) : null}
                      </article>
                    ))
                  )}
                </section>
                <section className="mt-5 space-y-2">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">Sit-ins ({sitInAbsences.length})</h3>
                  {sitInAbsences.length === 0 ? (
                    <EmptyState message="No sit-ins recorded for this day." />
                  ) : (
                    sitInAbsences.map((absence) => (
                      <SidePanelSitInCard key={absence.id} absence={absence} onViewStudent={setStudentAbsence} />
                    ))
                  )}
                </section>
              </motion.div>
            ) : (
              <motion.div key="absences" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: reduceMotion ? 0 : 0.1 }}>
                <section className="space-y-2">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">Absences ({absences.length})</h3>
                  {absences.length === 0 ? (
                    <EmptyState message="No absences this day." />
                  ) : (
                    absences.map((absence) => <SidePanelAbsenceRow key={absence.id} absence={absence} />)
                  )}
                </section>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </motion.aside>
    </AnimatePresence>
  );
}
