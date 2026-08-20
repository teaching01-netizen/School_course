import {
  useCallback,
  useContext,
  createContext,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  cloneElement,
  isValidElement,
  type MutableRefObject,
  type ReactElement,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import { cn } from "@/utils/cn";

/** Props the trigger element is expected to carry so injected ref/onClick
 *  attributes can merge with the consumer's own values. */
type TriggerElement = ReactElement<{
  ref?: Ref<HTMLElement>;
  onClick?: React.MouseEventHandler;
}>;

interface PopoverProps {
  /** The element that opens/closes the popover. Injected with ref, toggle
   *  onClick, and aria attributes; its own ref/onClick are preserved. */
  trigger: TriggerElement;
  /** Controlled open state. When omitted the popover manages its own. */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  align?: "start" | "center" | "end";
  role?: "dialog" | "menu" | "listbox" | "region";
  ariaLabel?: string;
  /** Move focus to the first focusable control on open (default true). */
  autoFocus?: boolean;
  contentClassName?: string;
  children: ReactNode;
}

const GAP = 4;

/**
 * Overlay tree: every open popover registers here with the id of its nearest
 * popover ancestor (via PopoverTreeContext). Because content is portaled to
 * <body>, DOM ancestry alone cannot tell a click inside a nested popover from
 * a click outside the whole tree — the registry makes that explicit:
 *  - an outside mousedown never closes a popover that has an open descendant
 *    containing the target;
 *  - Escape dismisses only the topmost open surface.
 */
interface PopoverNode {
  id: number;
  parentId: number | null;
  panelRef: RefObject<HTMLDivElement | null>;
  openRef: MutableRefObject<boolean>;
  close: (restoreFocus: boolean) => void;
}

const PopoverTreeContext = createContext<number | null>(null);
const popoverNodes = new Map<number, PopoverNode>();
let nextPopoverId = 1;

function openDescendantsOf(id: number): PopoverNode[] {
  const descendants: PopoverNode[] = [];
  for (const node of popoverNodes.values()) {
    if (node.id === id) continue;
    let cursor = node.parentId;
    while (cursor !== null) {
      if (cursor === id) {
        if (node.openRef.current) descendants.push(node);
        break;
      }
      cursor = popoverNodes.get(cursor)?.parentId ?? null;
    }
  }
  return descendants;
}

export function Popover({
  trigger,
  open: controlledOpen,
  onOpenChange,
  align = "start",
  role = "dialog",
  ariaLabel,
  autoFocus = true,
  contentClassName = "",
  children,
}: PopoverProps) {
  const [internalOpen, setInternalOpen] = useState(false);
  const open = controlledOpen ?? internalOpen;
  const setOpen = useCallback(
    (next: boolean) => {
      if (controlledOpen === undefined) setInternalOpen(next);
      onOpenChange?.(next);
    },
    [controlledOpen, onOpenChange],
  );

  const triggerRef = useRef<HTMLElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [position, setPosition] = useState<{ top: number; left: number } | null>(null);
  const triggerId = useId();
  const panelId = useId();

  const popoverIdRef = useRef(0);
  if (popoverIdRef.current === 0) popoverIdRef.current = nextPopoverId++;
  const parentNodeId = useContext(PopoverTreeContext);
  const openRef = useRef(open);
  openRef.current = open;

  // The trigger element is a per-render value; keep the injected ref stable by
  // resolving the original ref once per render and merging it with our own.
  const triggerChild = isValidElement(trigger)
    ? trigger
    : ((<button type="button">{trigger as ReactNode}</button>) as TriggerElement);
  const originalRef = triggerChild.props?.ref;

  const setTriggerRef = useCallback(
    (node: HTMLElement | null) => {
      triggerRef.current = node;
      if (typeof originalRef === "function") originalRef(node);
      else if (originalRef && typeof originalRef === "object") originalRef.current = node;
    },
    [originalRef],
  );

  const toggle = useCallback(() => setOpen(!open), [open, setOpen]);

  const close = useCallback(
    (restoreFocus: boolean) => {
      setOpen(false);
      if (restoreFocus) triggerRef.current?.focus();
    },
    [setOpen],
  );

  // Sync the anchor position on open and whenever geometry may have changed.
  useLayoutEffect(() => {
    if (!open) {
      setPosition(null);
      return;
    }
    const measure = () => {
      const triggerEl = triggerRef.current;
      const panelEl = panelRef.current;
      if (!triggerEl || !panelEl) return;
      const rect = triggerEl.getBoundingClientRect();
      const width = panelEl.offsetWidth;
      const height = panelEl.offsetHeight;
      const roomBelow = window.innerHeight - rect.bottom - GAP;
      const roomAbove = rect.top - GAP;
      const placeAbove = roomBelow < height && roomAbove > roomBelow;
      const top =
        placeAbove
          ? Math.max(GAP, rect.top - height - GAP)
          : Math.min(rect.bottom + GAP, window.innerHeight - height - GAP);
      let left =
        align === "center"
          ? rect.left + rect.width / 2 - width / 2
          : align === "end"
            ? rect.right - width
            : rect.left;
      left = Math.min(Math.max(GAP, left), window.innerWidth - width - GAP);
      setPosition({ top: Math.max(GAP, top), left });
    };
    measure();
    window.addEventListener("resize", measure);
    window.addEventListener("scroll", measure, true);
    let observer: ResizeObserver | null = null;
    if (typeof ResizeObserver !== "undefined" && panelRef.current) {
      observer = new ResizeObserver(measure);
      observer.observe(panelRef.current);
    }
    return () => {
      window.removeEventListener("resize", measure);
      window.removeEventListener("scroll", measure, true);
      observer?.disconnect();
    };
  }, [open, align]);

  // Light dismiss: outside mousedown closes, Escape closes and returns focus.
  useEffect(() => {
    if (!open) return;
    const handleMouseDown = (event: MouseEvent) => {
      const target = event.target as Node;
      if (triggerRef.current?.contains(target) || panelRef.current?.contains(target)) return;
      const insideDescendant = openDescendantsOf(popoverIdRef.current).some((node) =>
        node.panelRef.current?.contains(target),
      );
      if (insideDescendant) return;
      close(false);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      // The topmost open surface owns Escape; a popover with an open
      // descendant must not dismiss from under it.
      if (openDescendantsOf(popoverIdRef.current).length > 0) return;
      close(true);
    };
    document.addEventListener("mousedown", handleMouseDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handleMouseDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open, close]);

  // Join the overlay tree while open so ancestors can classify clicks and
  // key presses that land inside this panel.
  useEffect(() => {
    if (!open) return;
    const node: PopoverNode = {
      id: popoverIdRef.current,
      parentId: parentNodeId,
      panelRef,
      openRef,
      close,
    };
    popoverNodes.set(popoverIdRef.current, node);
    return () => {
      popoverNodes.delete(popoverIdRef.current);
    };
  }, [open, parentNodeId, close]);

  // Closing this popover takes open descendants with it. Descendants normally
  // unmount together with this panel; this is the safety net for closings
  // triggered outside the React tree (e.g. clicking the trigger to toggle).
  useEffect(() => {
    if (open) return;
    for (const node of openDescendantsOf(popoverIdRef.current)) node.close(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // Focus the first focusable control once the panel is on screen.
  useEffect(() => {
    if (!open || !autoFocus) return;
    const id = window.requestAnimationFrame
      ? window.requestAnimationFrame(() => focusFirstControl())
      : window.setTimeout(focusFirstControl, 0);
    return () => {
      if (window.cancelAnimationFrame) window.cancelAnimationFrame(id);
      else window.clearTimeout(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, autoFocus]);

  const focusFirstControl = () => {
    const panelEl = panelRef.current;
    if (!panelEl) return;
    const focusables = panelEl.querySelectorAll<HTMLElement>(
      'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
    );
    const first = Array.from(focusables).find((el) => el.tabIndex >= 0);
    if (first) first.focus();
    else panelEl.focus();
  };

  const triggerProps: Record<string, unknown> = {
    ref: setTriggerRef,
    id: triggerId,
    onClick: (event: React.MouseEvent) => {
      triggerChild.props?.onClick?.(event);
      if (!event.defaultPrevented) toggle();
    },
    "aria-expanded": open,
    "aria-controls": panelId,
  };
  if (role === "dialog" || role === "menu") triggerProps["aria-haspopup"] = role;
  if (triggerChild.type === "button") triggerProps.type = "button";

  const triggerEl = cloneElement(triggerChild, triggerProps);

  return (
    <>
      {triggerEl}
      {open &&
        createPortal(
          <div
            ref={panelRef}
            id={panelId}
            role={role}
            aria-label={ariaLabel}
            tabIndex={-1}
            data-state="open"
            className={cn(
              "animate-notion-popover-in fixed z-50 rounded-md border border-[var(--border-strong)] bg-white shadow-[0_12px_32px_rgba(0,0,0,0.14),0_1px_3px_rgba(0,0,0,0.12)] motion-reduce:animate-none",
              contentClassName,
            )}
            style={{
              top: position?.top ?? 0,
              left: position?.left ?? 0,
              visibility: position ? "visible" : "hidden",
            }}
          >
            <PopoverTreeContext.Provider value={popoverIdRef.current}>
              {children}
            </PopoverTreeContext.Provider>
          </div>,
          document.body,
        )}
    </>
  );
}