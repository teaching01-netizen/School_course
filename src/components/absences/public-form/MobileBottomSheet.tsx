import { useEffect, useId, useLayoutEffect, useRef, type ReactNode, type RefObject } from "react";

export type MobileBottomSheetProps = {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  restoreFocusRef?: RefObject<HTMLElement | null>;
};

export default function MobileBottomSheet({ open, title, onClose, children, restoreFocusRef }: MobileBottomSheetProps) {
  const dialogRef = useRef<HTMLDialogElement | null>(null);
  const titleId = useId();
  const onCloseRef = useRef(onClose);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useLayoutEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog || !open) return;

    const previousFocus = restoreFocusRef?.current ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null);
    const focusInitialControl = () => {
      const target = dialog.querySelector<HTMLElement>(
        '[data-sheet-initial-focus], button:not([aria-label="Close sheet"]), input, select, textarea, [tabindex="0"]',
      );
      target?.focus();
    };
    const handleCancel = (event: Event) => {
      event.preventDefault();
      onCloseRef.current();
    };
    const handleBackdropClick = (event: MouseEvent) => {
      const bounds = dialog.getBoundingClientRect();
      const clickedOutside = event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom;
      if (clickedOutside) onCloseRef.current();
    };
    const handleFocusIn = (event: FocusEvent) => {
      if (!dialog.contains(event.target as Node)) focusInitialControl();
    };

    dialog.addEventListener("cancel", handleCancel);
    dialog.addEventListener("click", handleBackdropClick);
    document.addEventListener("focusin", handleFocusIn);
    dialog.removeAttribute("open");
    try {
      if (typeof dialog.showModal === "function") dialog.showModal();
      else dialog.setAttribute("open", "");
    } catch {
      dialog.setAttribute("open", "");
    }
    focusInitialControl();
    window.requestAnimationFrame(focusInitialControl);

    return () => {
      dialog.removeEventListener("cancel", handleCancel);
      dialog.removeEventListener("click", handleBackdropClick);
      document.removeEventListener("focusin", handleFocusIn);
      if (dialog.open) {
        try { dialog.close(); } catch { dialog.removeAttribute("open"); }
      }
      const restoreFocus = () => {
        if (restoreFocusRef?.current && document.contains(restoreFocusRef.current)) restoreFocusRef.current.focus();
        else if (previousFocus && document.contains(previousFocus)) previousFocus.focus();
      };
      restoreFocus();
      window.requestAnimationFrame(restoreFocus);
    };
  }, [open, restoreFocusRef]);

  if (!open) return null;

  return (
    <dialog ref={dialogRef} open role="dialog" aria-modal="true" aria-labelledby={titleId} className="absence-mobile-sheet">
      <div className="absence-mobile-sheet__surface">
        <div className="absence-mobile-sheet__handle" aria-hidden="true" />
        <div className="flex items-start justify-between gap-4">
          <h2 id={titleId} className="text-lg font-semibold text-[var(--color-wi-text)]">{title}</h2>
          <button type="button" onClick={() => onCloseRef.current()} aria-label="Close sheet" className="min-h-11 min-w-11 rounded-full text-2xl leading-none text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]">
            <span aria-hidden="true">×</span>
          </button>
        </div>
        <div className="mt-5 max-h-[min(62dvh,34rem)] overflow-y-auto pr-1">{children}</div>
      </div>
    </dialog>
  );
}
