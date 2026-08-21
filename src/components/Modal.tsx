import { type ReactNode, useId, useRef } from 'react';
import { useDialogModal } from '@/hooks/useDialogModal';

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
  const { close } = useDialogModal(dialogRef, { onClose, closeOnBackdrop: true });

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
          <button type="button" aria-label="Close dialog" onClick={() => void close()} className="text-[var(--color-wi-faint)] hover:text-[var(--color-wi-text)] text-xl leading-none p-1">&times;</button>
        </div>
        <div className="p-4 overflow-y-auto max-h-[70vh]">{children}</div>
        {footer && <div className="flex justify-end gap-2 px-4 py-3 border-t border-t-[var(--color-wi-line)] bg-[var(--color-wi-callout)] rounded-b-lg">{footer}</div>}
      </div>
    </dialog>
  );
}
