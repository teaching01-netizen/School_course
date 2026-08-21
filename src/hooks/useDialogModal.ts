import { useCallback, useEffect, useLayoutEffect, useRef } from "react";
import type { RefObject } from "react";

type Opts = {
  onClose?: () => void;
  closeOnBackdrop?: boolean;
};

export function useDialogModal(
  ref: RefObject<HTMLDialogElement | null>,
  opts: Opts = {},
) {
  const onCloseRef = useRef(opts.onClose);
  const triggerRef = useRef<HTMLElement | null>(null);
  const closeInFlightRef = useRef(false);

  useEffect(() => {
    onCloseRef.current = opts.onClose;
  }, [opts.onClose]);

  const close = useCallback(async () => {
    const dialog = ref.current;
    if (!dialog) {
      onCloseRef.current?.();
      return;
    }
    if (closeInFlightRef.current) return;
    closeInFlightRef.current = true;
    dialog.dataset.closing = "true";
    await Promise.race([
      new Promise<void>((resolve) => {
        const onEnd = () => {
          dialog.removeEventListener("animationend", onEnd);
          resolve();
        };
        dialog.addEventListener("animationend", onEnd, { once: true } as AddEventListenerOptions);
      }),
      new Promise<void>((resolve) => setTimeout(resolve, 200)),
    ]);
    delete (dialog.dataset as DOMStringMap).closing;
    try {
      dialog.close();
    } catch {
      // already closed
    }
    const trigger = triggerRef.current;
    if (trigger && document.contains(trigger)) {
      trigger.focus();
    } else {
      try {
        (document.body as HTMLElement).focus?.();
      } catch {}
    }
    closeInFlightRef.current = false;
    onCloseRef.current?.();
  }, [ref]);

  const open = useCallback(() => {
    const dialog = ref.current;
    if (!dialog) return;
    triggerRef.current =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialog.removeAttribute("open");
    try {
      dialog.showModal();
      if (Object.prototype.hasOwnProperty.call(HTMLDialogElement.prototype, "closedBy")) {
        try {
          (dialog as unknown as { closedBy: string }).closedBy = "any";
        } catch {}
      }
    } catch {
      try {
        dialog.setAttribute("open", "");
      } catch {}
      return;
    }
    const firstFocusable = dialog.querySelector<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    );
    firstFocusable?.focus();
  }, [ref]);

  useLayoutEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;

    open();

    const hasNativeClosedBy = Object.prototype.hasOwnProperty.call(
      HTMLDialogElement.prototype,
      "closedBy",
    );

    let handleBackdropClick: ((e: MouseEvent) => void) | undefined;

    if (!hasNativeClosedBy && opts.closeOnBackdrop !== false) {
      handleBackdropClick = (e: MouseEvent) => {
        const path: EventTarget[] = (e as unknown as { composedPath?: () => EventTarget[] }).composedPath?.() ?? [];
        const isBackdropViaPath = path.length ? path[0] === dialog : e.target === dialog;
        const rect = dialog.getBoundingClientRect();
        const isBackdropViaRect =
          e.clientX < rect.left ||
          e.clientX > rect.right ||
          e.clientY < rect.top ||
          e.clientY > rect.bottom;
        const isBackdrop = isBackdropViaPath || isBackdropViaRect;
        if (!isBackdrop) return;
        void close();
      };
      dialog.addEventListener("click", handleBackdropClick);
    }

    const handleNativeClose = () => {
      if (dialog.dataset.closing === "true") return;
      const trigger = triggerRef.current;
      if (trigger && document.contains(trigger)) {
        trigger.focus();
      } else {
        try {
          (document.body as HTMLElement).focus?.();
        } catch {}
      }
      onCloseRef.current?.();
    };
    dialog.addEventListener("close", handleNativeClose);

    return () => {
      dialog.removeEventListener("close", handleNativeClose);
      if (handleBackdropClick) dialog.removeEventListener("click", handleBackdropClick);
      if (dialog.open) {
        try {
          dialog.close();
        } catch {}
        const trigger = triggerRef.current;
        if (trigger && document.contains(trigger)) {
          trigger.focus();
        } else {
          try {
            (document.body as HTMLElement).focus?.();
          } catch {}
        }
      }
    };
    // hasNativeClosedBy computed inside effect so test mocks that mutate prototype are observed
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ref, opts.closeOnBackdrop, open, close]);

  return { open, close };
}
