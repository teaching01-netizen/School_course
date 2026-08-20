import { useState, useRef, useCallback, useEffect, useId } from "react";
import { Info } from "lucide-react";

type TooltipProps = {
  content: string;
  className?: string;
};

export function Tooltip({ content, className = "" }: TooltipProps) {
  const [visible, setVisible] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const delayRef = useRef<number | null>(null);
  const tooltipId = useId();

  // Pointer users get a Notion-style 150ms delay; keyboard focus shows instantly.
  const showHover = useCallback(() => {
    if (delayRef.current !== null) window.clearTimeout(delayRef.current);
    delayRef.current = window.setTimeout(() => setVisible(true), 150);
  }, []);

  const showFocus = useCallback(() => {
    if (delayRef.current !== null) window.clearTimeout(delayRef.current);
    delayRef.current = null;
    setVisible(true);
  }, []);

  const hide = useCallback(() => {
    if (delayRef.current !== null) window.clearTimeout(delayRef.current);
    delayRef.current = null;
    setVisible(false);
  }, []);

  useEffect(() => {
    return () => {
      if (delayRef.current !== null) window.clearTimeout(delayRef.current);
    };
  }, []);

  return (
    <span className={`relative inline-flex ${className}`}>
      <button
        ref={triggerRef}
        type="button"
        className="inline-flex items-center justify-center w-4 h-4 rounded-full text-[var(--color-wi-faint)] hover:text-[var(--color-wi-text)] hover:bg-[var(--color-wi-row-alt)] transition-colors"
        onMouseEnter={showHover}
        onMouseLeave={hide}
        onFocus={showFocus}
        onBlur={hide}
        aria-describedby={visible ? tooltipId : undefined}
      >
        <Info className="w-3.5 h-3.5" />
      </button>
      {visible && (
        <span
          id={tooltipId}
          role="tooltip"
          className="animate-notion-tooltip-in absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2.5 py-1.5 text-xs text-white bg-[var(--color-wi-text)] rounded-sm shadow-lg whitespace-nowrap max-w-[240px] text-wrap z-50 pointer-events-none motion-reduce:animate-none"
        >
          {content}
          <span className="absolute top-full left-1/2 -translate-x-1/2 border-4 border-transparent border-t-[var(--color-wi-text)]" />
        </span>
      )}
    </span>
  );
}

export default Tooltip;