import { useRef, useCallback } from "react";
import { motion, useReducedMotion } from "framer-motion";
import clsx from "clsx";

type CalendarCellStatus = "available" | "absent" | "cover";

type CalendarCellProps = {
  sessionId: string;
  startTime: string;
  endTime: string;
  status: CalendarCellStatus;
  onToggleAbsent: (sessionId: string) => void;
  onToggleCover: (sessionId: string) => void;
};

const CLICK_DEBOUNCE_MS = 250;

export default function CalendarCell({
  sessionId,
  startTime,
  endTime,
  status,
  onToggleAbsent,
  onToggleCover,
}: CalendarCellProps) {
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const longPressFired = useRef(false);
  const clickTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reduceMotion = useReducedMotion();

  const ariaLabel = `${sessionId} ${startTime}–${endTime} ${status}`;
  const ariaPressed = status !== "available";

  const clearLongPress = useCallback(() => {
    if (longPressTimer.current) {
      clearTimeout(longPressTimer.current);
      longPressTimer.current = null;
    }
  }, []);

  const clearClickTimer = useCallback(() => {
    if (clickTimer.current) {
      clearTimeout(clickTimer.current);
      clickTimer.current = null;
    }
  }, []);

  const handlePointerDown = useCallback(() => {
    longPressFired.current = false;
    longPressTimer.current = setTimeout(() => {
      longPressFired.current = true;
      onToggleCover(sessionId);
    }, 500);
  }, [onToggleCover, sessionId]);

  const handlePointerUp = useCallback(() => {
    clearLongPress();
  }, [clearLongPress]);

  const handlePointerLeave = useCallback(() => {
    clearLongPress();
  }, [clearLongPress]);

  const handlePointerCancel = useCallback(() => {
    clearLongPress();
  }, [clearLongPress]);

  const handleClick = useCallback(() => {
    if (longPressFired.current) {
      longPressFired.current = false;
      return;
    }
    clearClickTimer();
    clickTimer.current = setTimeout(() => {
      clickTimer.current = null;
      onToggleAbsent(sessionId);
    }, CLICK_DEBOUNCE_MS);
  }, [clearClickTimer, onToggleAbsent, sessionId]);

  const handleDoubleClick = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      clearClickTimer();
      clearLongPress();
      onToggleCover(sessionId);
    },
    [clearClickTimer, clearLongPress, onToggleCover, sessionId],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && e.shiftKey) {
        e.preventDefault();
        onToggleCover(sessionId);
      } else if (e.key === "Enter") {
        e.preventDefault();
        onToggleAbsent(sessionId);
      }
    },
    [onToggleAbsent, onToggleCover, sessionId],
  );

  return (
    <motion.div
      role="gridcell"
      aria-label={ariaLabel}
      aria-pressed={ariaPressed}
      tabIndex={0}
      whileTap={reduceMotion ? undefined : { scale: 0.97 }}
      transition={{ type: "spring", stiffness: 400, damping: 25 }}
      className={clsx(
        "rounded-sm border px-3 py-2 text-sm font-medium transition-colors cursor-pointer select-none",
        status === "available" && "bg-gray-100 border-gray-300 text-gray-700",
        status === "absent" && "bg-green-100 border-green-300 text-green-800",
        status === "cover" && "bg-amber-100 border-amber-300 text-amber-800",
      )}
      onPointerDown={handlePointerDown}
      onPointerUp={handlePointerUp}
      onPointerLeave={handlePointerLeave}
      onPointerCancel={handlePointerCancel}
      onClick={handleClick}
      onDoubleClick={handleDoubleClick}
      onKeyDown={handleKeyDown}
    >
      {startTime}–{endTime}
    </motion.div>
  );
}
