import { useEffect, useRef, type CSSProperties, type ReactNode } from "react";
import { useVisualViewport } from "@/hooks/useVisualViewport";

type AbsenceAppShellProps = {
  children: ReactNode;
  header: ReactNode;
  footer: ReactNode;
};

export default function AbsenceAppShell({ children, header, footer }: AbsenceAppShellProps) {
  const viewport = useVisualViewport();
  const mainRef = useRef<HTMLElement | null>(null);
  const headingRef = useRef<HTMLElement | null>(null);
  const announceRef = useRef<HTMLDivElement | null>(null);
  const style = {
    "--absence-visual-viewport-height": `${viewport.height}px`,
    "--absence-visual-viewport-offset": `${viewport.offsetTop}px`,
  } as CSSProperties;

  useEffect(() => {
    const main = mainRef.current;
    if (!main) return;
    const heading = (main.querySelector("h1, [data-absence-heading]") as HTMLElement | null) ?? main;
    headingRef.current = heading;
    const label = heading.textContent?.trim() ?? "";
    if (announceRef.current) announceRef.current.textContent = label;
    heading.tabIndex = -1;
    heading.focus({ preventScroll: true });
    try {
      heading.scrollIntoView({ behavior: "auto", block: "start" });
    } catch {}
  }, [children]);

  useEffect(() => {
    const main = mainRef.current;
    if (!main || !viewport.keyboardLikelyOpen) return;

    const ensureFocusedControlVisible = (target: EventTarget | null) => {
      if (!(target instanceof HTMLElement) || !main.contains(target)) return;
      const adjustScroll = () => {
        const mainRect = main.getBoundingClientRect();
        const footerRect = main.parentElement?.querySelector<HTMLElement>(".absence-app-shell__footer")?.getBoundingClientRect();
        const visibleTop = Math.max(mainRect.top, viewport.offsetTop);
        const visibleBottom = Math.min(
          mainRect.bottom,
          footerRect?.top ?? Number.POSITIVE_INFINITY,
          viewport.offsetTop + viewport.height,
        );
        const targetRect = target.getBoundingClientRect();
        const delta = targetRect.bottom > visibleBottom
          ? targetRect.bottom - visibleBottom
          : targetRect.top < visibleTop
            ? targetRect.top - visibleTop
            : 0;
        if (delta !== 0) {
          const nextScrollTop = Math.max(0, Math.min(main.scrollHeight - main.clientHeight, main.scrollTop + delta));
          main.scrollTop = nextScrollTop;
          main.scrollTo({ top: nextScrollTop, behavior: "auto" });
        }
      };
      window.requestAnimationFrame(() => {
        adjustScroll();
        window.requestAnimationFrame(adjustScroll);
      });
    };

    const handleFocusIn = (event: FocusEvent) => ensureFocusedControlVisible(event.target);
    main.addEventListener("focusin", handleFocusIn);
    ensureFocusedControlVisible(document.activeElement);

    return () => main.removeEventListener("focusin", handleFocusIn);
  }, [viewport.height, viewport.keyboardLikelyOpen, viewport.offsetTop]);

  return (
    <div
      className="absence-app-shell"
      data-keyboard-open={viewport.keyboardLikelyOpen ? "true" : "false"}
      style={style}
    >
      <div ref={announceRef} aria-live="polite" aria-atomic="true" className="sr-only" />
      <header className="absence-app-shell__header">{header}</header>
      <main ref={mainRef} id="absence-form-content" tabIndex={-1} className="absence-app-shell__main">
        {children}
      </main>
      <footer className="absence-app-shell__footer">{footer}</footer>
    </div>
  );
}
