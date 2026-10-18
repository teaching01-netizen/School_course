import { useEffect, useId, useLayoutEffect, useRef, type CSSProperties, type ReactNode, type RefObject } from "react";
import { useVisualViewport } from "@/hooks/useVisualViewport";

export type MobileBottomSheetProps = {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
  restoreFocusRef?: RefObject<HTMLElement | null>;
  publicForm?: boolean;
  description?: string;
  summary?: ReactNode;
  toolbar?: ReactNode;
  footer?: ReactNode;
};

export default function MobileBottomSheet(props: MobileBottomSheetProps) {
  return props.open ? <OpenMobileBottomSheet {...props} /> : null;
}

function OpenMobileBottomSheet({ title, onClose, children, restoreFocusRef, publicForm = false, description, summary, toolbar, footer }: MobileBottomSheetProps) {
  const viewport = useVisualViewport();
  const dialogRef = useRef<HTMLDialogElement | null>(null);
  const titleId = useId();
  const descriptionId = useId();
  const onCloseRef = useRef(onClose);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useLayoutEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;

    const previousFocus = restoreFocusRef?.current ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null);
    const focusInitialControl = () => {
      const target = dialog.querySelector<HTMLElement>('[data-sheet-initial-focus]') ?? dialog.querySelector<HTMLElement>(
        'button:not([aria-label="Close sheet"]), input, select, textarea, [tabindex="0"]',
      );
      target?.focus({ preventScroll: true });
    };
    const handleCancel = (event: Event) => {
      event.preventDefault();
      onCloseRef.current();
    };
    const handleBackdropClick = (event: MouseEvent) => {
      const bounds = dialog.getBoundingClientRect();
      const clickedOutside = event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom;
      if (clickedOutside || event.target === dialog) onCloseRef.current();
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
    const initialFrame = window.requestAnimationFrame(() => {
      if (!dialog.contains(document.activeElement)) focusInitialControl();
    });

    return () => {
      dialog.removeEventListener("cancel", handleCancel);
      window.cancelAnimationFrame(initialFrame);
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
    };
  }, [restoreFocusRef]);

  const sheetHeader = (
        <div className="absence-mobile-sheet__header" tabIndex={publicForm ? 0 : undefined} role={publicForm ? "region" : undefined} aria-label={publicForm ? `${title} details` : undefined}>
          <div className="flex items-start justify-between gap-3">
            <h2 id={titleId} tabIndex={publicForm ? -1 : undefined} data-sheet-initial-focus={publicForm ? true : undefined} className="min-w-0 text-lg font-semibold text-[var(--color-wi-text)] outline-none">{title}</h2>
            {!publicForm ? <button type="button" onClick={() => onCloseRef.current()} aria-label="Close sheet" className="min-h-11 min-w-11 shrink-0 rounded-lg px-2 text-base font-medium text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"><span aria-hidden="true">×</span></button> : null}
          </div>
          {description ? <p id={descriptionId} className="mt-2 text-sm text-[var(--color-wi-text-light)]">{description}</p> : null}
          {summary}
        </div>
  );

  return (
    <dialog
      ref={dialogRef} open role="dialog" aria-modal="true" aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      data-form-mode={publicForm ? "public" : undefined}
      data-keyboard-open={viewport.keyboardLikelyOpen || undefined}
      className="absence-mobile-sheet"
      style={{ "--absence-sheet-height": `${viewport.height}px`, "--absence-sheet-offset": `${viewport.offsetTop}px` } as CSSProperties}
    >
      <div className="absence-mobile-sheet__surface">
        {!publicForm ? <div className="absence-mobile-sheet__handle" aria-hidden="true" /> : null}
        {publicForm ? <div className="absence-mobile-sheet__navigation">
          <button type="button" onClick={() => onCloseRef.current()} className="absence-mobile-sheet__cancel min-h-11 min-w-11 rounded-lg px-2 text-base font-medium text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]">Cancel</button>
        </div> : null}
        {publicForm ? sheetHeader : null}
        <div className="absence-mobile-sheet__content">
        {!publicForm ? sheetHeader : null}
        {toolbar ? <div className="absence-mobile-sheet__toolbar">{toolbar}</div> : null}
        <div className="absence-mobile-sheet__body mt-5 max-h-[min(62dvh,34rem)] overflow-y-auto pr-1">{children}</div>
        </div>
        {footer ? <div className="absence-mobile-sheet__footer">{footer}</div> : null}
      </div>
    </dialog>
  );
}
