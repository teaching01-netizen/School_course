import { type ReactNode, useEffect, useRef, useCallback } from 'react';

type ModalSize = "sm" | "md" | "lg" | "xl" | "full";

const sizeMap: Record<ModalSize, string> = {
  sm: "max-w-sm",
  md: "max-w-md",
  lg: "max-w-lg",
  xl: "max-w-xl",
  full: "max-w-4xl",
};

interface ModalProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
  size?: ModalSize;
  maxWidth?: string;
  closeOnOverlay?: boolean;
  closeOnEscape?: boolean;
}

export default function Modal({ title, children, onClose, footer, size = "md", maxWidth, closeOnOverlay = true, closeOnEscape = true }: ModalProps) {
  const panelRef = useRef<HTMLDivElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);
  const titleId = "modal-title";

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  const handleEscape = useCallback((e: KeyboardEvent) => {
    if (closeOnEscape && e.key === "Escape") onCloseRef.current();
  }, [closeOnEscape]);

  useEffect(() => {
    previousFocus.current = document.activeElement as HTMLElement;
    document.addEventListener("keydown", handleEscape);
    document.body.style.overflow = "hidden";

    // auto-focus first focusable element
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

  // Focus trap
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "Tab") return;
    const panel = panelRef.current;
    if (!panel) return;
    const focusable = panel.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
    );
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/50 pt-[10vh] pb-8 px-4 animate-modal-overlay-enter"
      onClick={(e) => { if (closeOnOverlay && e.target === e.currentTarget) onClose(); }}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
    >
      <div
        ref={panelRef}
        className={`bg-white rounded-sm shadow-xl w-full ${maxWidth ?? sizeMap[size]} animate-modal-enter`}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200">
          <h3 id={titleId} className="text-base font-semibold text-[var(--color-wi-text)]">{title}</h3>
          <button onClick={onClose} className="text-gray-500 hover:text-gray-700 text-xl leading-none p-1" aria-label="Close dialog">&times;</button>
        </div>
        <div className="p-4 overflow-y-auto max-h-[70vh]">{children}</div>
        {footer && <div className="flex justify-end gap-2 px-4 py-3 border-t border-gray-200 bg-gray-50">{footer}</div>}
      </div>
    </div>
  );
}
