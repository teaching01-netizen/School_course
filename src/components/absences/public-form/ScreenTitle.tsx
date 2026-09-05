import type { ReactNode } from "react";

interface ScreenTitleProps {
  children: ReactNode;
  /** Extra classes for one-off spacing (e.g. `mt-5`, `mt-6`). Defaults to none. */
  className?: string;
  /** Passthrough for the success screen's programmatic-focus target. */
  id?: string;
  tabIndex?: number;
}

/**
 * The public absence flow's screen title: one editorial voice (28px bold,
 * tight tracking) shared by every step — identify, confirm, classes,
 * make-up, reason, review, resume, and success — so the wizard reads as a
 * single calm report instead of nine loosely-related pages.
 */
export default function ScreenTitle({ children, className = "", id, tabIndex }: ScreenTitleProps) {
  return (
    <h1
      id={id}
      tabIndex={tabIndex}
      className={`text-balance text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)] ${className} ${tabIndex !== undefined ? "outline-none" : ""}`}
    >
      {children}
    </h1>
  );
}

interface ScreenSubtitleProps {
  children: ReactNode;
  className?: string;
}

/** The supporting line under a screen title. Kept separate so copy edits
 *  never drift the 17px relaxed rhythm one screen at a time. */
export function ScreenSubtitle({ children, className = "" }: ScreenSubtitleProps) {
  return (
    <p className={`mt-2 text-[17px] leading-relaxed text-[var(--color-wi-text-light)] ${className}`}>
      {children}
    </p>
  );
}
