import { type ReactNode, useEffect, useLayoutEffect, useId, useRef } from 'react';

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
}

export default function Modal({ title, children, onClose, footer, size = "md", maxWidth }: ModalProps) {
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
      className="modal-base"
    >
      <div className={`animate-notion-dialog-in bg-white rounded-lg shadow-[0_1px_2px_rgba(0,0,0,0.04),0_8px_30px_rgba(0,0,0,0.05)] w-full ${maxWidth ?? sizeMap[size]} motion-reduce:animate-none`}>
        <div className="flex items-center justify-between px-4 py-3 border-b border-b-[var(--color-wi-line)]">
          <h3 id={titleId} className="text-base font-semibold text-[var(--color-wi-text)]">{title}</h3>
          <button onClick={() => onCloseRef.current()} className="text-[var(--color-wi-faint)] hover:text-[var(--color-wi-text)] text-xl leading-none p-1" aria-label="Close dialog">&times;</button>
        </div>
        <div className="p-4 overflow-y-auto max-h-[70vh]">{children}</div>
        {footer && <div className="flex justify-end gap-2 px-4 py-3 border-t border-t-[var(--color-wi-line)] bg-[var(--color-wi-callout)] rounded-b-lg">{footer}</div>}
      </div>
    </dialog>
  );
}