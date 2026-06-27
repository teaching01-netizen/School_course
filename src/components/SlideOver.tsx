import { type ReactNode, useEffect, useRef } from "react";

interface SlideOverProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
}

export default function SlideOver({ title, children, onClose, footer }: SlideOverProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const titleId = "slideover-title";
  const onCloseRef = useRef(onClose);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;

    dialog.showModal();

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
        dialog.close();
      };
    }

    return () => { dialog.close(); };
  }, []);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    const handleClose = () => onCloseRef.current();
    dialog.addEventListener("close", handleClose);
    return () => dialog.removeEventListener("close", handleClose);
  }, []);

  return (
    <dialog
      ref={dialogRef}
      closedby="any"
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      className="slideover-base"
    >
      <div className="relative w-full max-w-md bg-white shadow-xl h-full overflow-y-auto">
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

        <div className="p-4">{children}</div>

        {footer && (
          <div className="sticky bottom-0 bg-white border-t border-gray-200 px-4 py-3">
            {footer}
          </div>
        )}
      </div>
    </dialog>
  );
}
