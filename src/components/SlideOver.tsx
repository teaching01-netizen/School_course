import { type ReactNode, useId, useRef } from "react";
import { useDialogModal } from "@/hooks/useDialogModal";

interface SlideOverProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
}

export default function SlideOver({ title, children, onClose, footer }: SlideOverProps) {
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
      className="slideover-base"
    >
      <div className="animate-notion-sidepeek-in relative w-full max-w-md bg-white shadow-[0_1px_2px_rgba(0,0,0,0.04),0_8px_30px_rgba(0,0,0,0.05)] h-full overflow-y-auto rounded-l-[8px] motion-reduce:animate-none">
        <div className="flex items-center justify-between px-4 py-3 border-b border-b-[var(--color-wi-line)] sticky top-0 bg-white z-10">
          <h3 id={titleId} className="text-base font-semibold text-[var(--color-wi-text)]">
            {title}
          </h3>
          <button
            type="button"
            aria-label="Close panel"
            onClick={() => void close()}
            className="text-[var(--color-wi-faint)] hover:text-[var(--color-wi-text)] text-xl leading-none p-1"
          >
            &times;
          </button>
        </div>

        <div className="p-4">{children}</div>

        {footer && (
          <div className="sticky bottom-0 bg-white border-t border-t-[var(--color-wi-line)] px-4 py-3">
            {footer}
          </div>
        )}
      </div>
    </dialog>
  );
}
