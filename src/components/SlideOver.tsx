import { type ReactNode, useEffect, useLayoutEffect, useId, useRef } from "react";

interface SlideOverProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
}

export default function SlideOver({ title, children, onClose, footer }: SlideOverProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const onCloseRef = useRef(onClose);
  const suppressCloseRef = useRef(false);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useLayoutEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;

    dialog.removeAttribute("open");

    try { dialog.showModal(); } catch { return; }

    if (!('closedBy' in HTMLDialogElement.prototype)) {
      const handleBackdropClick = (e: MouseEvent) => {
        const rect = dialog.getBoundingClientRect();
        const isOutside = (
          e.clientX < rect.left || e.clientX > rect.right ||
          e.clientY < rect.top || e.clientY > rect.bottom
        );
        if (isOutside) onCloseRef.current();
      };
      dialog.addEventListener("click", handleBackdropClick);
      return () => {
        dialog.removeEventListener("click", handleBackdropClick);
        suppressCloseRef.current = true;
        dialog.close();
      };
    }

    return () => {
      suppressCloseRef.current = true;
      dialog.close();
    };
  }, []);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    const handleClose = () => {
      if (suppressCloseRef.current) {
        suppressCloseRef.current = false;
        return;
      }
      onCloseRef.current();
    };
    dialog.addEventListener("close", handleClose);
    return () => dialog.removeEventListener("close", handleClose);
  }, []);

  return (
    <dialog
      ref={dialogRef}
      open
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      className="slideover-base"
    >
      <div className="animate-notion-sidepeek-in relative w-full max-w-md bg-white shadow-[0_1px_2px_rgba(0,0,0,0.04),0_8px_30px_rgba(0,0,0,0.05)] h-full overflow-y-auto rounded-l-[8px] motion-reduce:animate-none">
        <div className="flex items-center justify-between px-4 py-3 border-b var(--color-wi-line) sticky top-0 bg-white z-10">
          <h3 id={titleId} className="text-base font-semibold text-[var(--color-wi-text)]">
            {title}
          </h3>
          <button
            onClick={() => onCloseRef.current()}
            className="text-[var(--color-wi-faint)] hover:text-[var(--color-wi-text)] text-xl leading-none p-1"
            aria-label="Close panel"
          >
            &times;
          </button>
        </div>

        <div className="p-4">{children}</div>

        {footer && (
          <div className="sticky bottom-0 bg-white border-t var(--color-wi-line) px-4 py-3">
            {footer}
          </div>
        )}
      </div>
    </dialog>
  );
}