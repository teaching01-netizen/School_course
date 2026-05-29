import { type ReactNode, useEffect, useRef, useCallback } from "react";

interface SlideOverProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
}

export default function SlideOver({ title, children, onClose, footer }: SlideOverProps) {
  const panelRef = useRef<HTMLDivElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);
  const titleId = "slideover-title";

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  const handleEscape = useCallback((e: KeyboardEvent) => {
    if (e.key === "Escape") onCloseRef.current();
  }, []);

  useEffect(() => {
    previousFocus.current = document.activeElement as HTMLElement;
    document.addEventListener("keydown", handleEscape);
    document.body.style.overflow = "hidden";

    const panel = panelRef.current;
    if (panel) {
      const focusable = panel.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      );
      if (focusable.length > 0) focusable[0].focus();
    }

    return () => {
      document.removeEventListener("keydown", handleEscape);
      document.body.style.overflow = "";
      previousFocus.current?.focus();
    };
  }, [handleEscape]);

  return (
    <div
      className="fixed inset-0 z-50 flex justify-end"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/30 animate-fade-in" />

      {/* Slide panel */}
      <div
        ref={panelRef}
        className="relative w-full max-w-md bg-white shadow-xl h-full overflow-y-auto animate-slide-in-right"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200 sticky top-0 bg-white z-10">
          <h3 id={titleId} className="text-base font-semibold text-[var(--color-wi-text)]">
            {title}
          </h3>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-700 text-xl leading-none p-1"
            aria-label="Close panel"
          >
            &times;
          </button>
        </div>

        {/* Body */}
        <div className="p-4">{children}</div>

        {/* Footer */}
        {footer && (
          <div className="sticky bottom-0 bg-white border-t border-gray-200 px-4 py-3">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}
