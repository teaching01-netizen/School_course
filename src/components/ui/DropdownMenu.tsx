import { useState, useRef, useEffect, useCallback, useId, useLayoutEffect, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { MoreVertical } from "lucide-react";

interface DropdownMenuItem {
  label: string;
  onClick: () => void;
  danger?: boolean;
  disabled?: boolean;
}

interface DropdownMenuProps {
  items: DropdownMenuItem[];
  triggerClassName?: string;
  /** When provided, replaces the default icon trigger with custom content (e.g. a user chip). */
  trigger?: ReactNode;
}

export function DropdownMenu({ items, triggerClassName = "", trigger }: DropdownMenuProps) {
  const [open, setOpen] = useState(false);
  const [menuPosition, setMenuPosition] = useState<{ top: number; left: number } | null>(null);
  const [openFocus, setOpenFocus] = useState<"first" | "last" | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const triggerId = useId();
  const menuId = useId();

  const close = useCallback((restoreFocus = false) => {
    setOpen(false);
    setOpenFocus(null);
    if (restoreFocus) triggerRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      const target = e.target as Node;
      if (!ref.current?.contains(target) && !menuRef.current?.contains(target)) close();
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open, close]);

  useEffect(() => {
    if (!open) return;
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") close(true);
    };
    document.addEventListener("keydown", handleEscape);
    return () => document.removeEventListener("keydown", handleEscape);
  }, [open, close]);

  useEffect(() => {
    if (!open || !openFocus) return;
    const frame = window.requestAnimationFrame(() => {
      const enabledItems = itemRefs.current.filter((item): item is HTMLButtonElement => Boolean(item && !item.disabled));
      const item = openFocus === "first" ? enabledItems[0] : enabledItems.at(-1);
      item?.focus();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [open, openFocus, items.length]);

  useLayoutEffect(() => {
    if (!open) return;
    const updatePlacement = () => {
      const trigger = triggerRef.current;
      const menu = menuRef.current;
      if (!trigger || !menu) return;
      const rect = trigger.getBoundingClientRect();
      const gap = 4;
      const width = menu.offsetWidth;
      const height = menu.offsetHeight;
      const roomBelow = window.innerHeight - rect.bottom - gap;
      const roomAbove = rect.top - gap;
      const top = roomBelow < height && roomAbove > roomBelow
        ? rect.top - height - gap
        : Math.min(rect.bottom + gap, window.innerHeight - height - gap);
      const left = Math.min(Math.max(gap, rect.right - width), window.innerWidth - width - gap);
      setMenuPosition({ top: Math.max(gap, top), left });
    };
    updatePlacement();
    window.addEventListener("resize", updatePlacement);
    window.addEventListener("scroll", updatePlacement, true);
    return () => {
      window.removeEventListener("resize", updatePlacement);
      window.removeEventListener("scroll", updatePlacement, true);
    };
  }, [open, items.length]);

  const handleTriggerKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setOpen(true);
      setOpenFocus(event.key === "ArrowDown" ? "first" : "last");
    }
  };

  const handleMenuKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const enabledItems = itemRefs.current.filter((item): item is HTMLButtonElement => Boolean(item && !item.disabled));
    if (enabledItems.length === 0) return;

    const currentIndex = enabledItems.indexOf(document.activeElement as HTMLButtonElement);
    let nextIndex: number | null = null;

    if (event.key === "ArrowDown") nextIndex = currentIndex < enabledItems.length - 1 ? currentIndex + 1 : 0;
    if (event.key === "ArrowUp") nextIndex = currentIndex > 0 ? currentIndex - 1 : enabledItems.length - 1;
    if (event.key === "Home") nextIndex = 0;
    if (event.key === "End") nextIndex = enabledItems.length - 1;

    if (nextIndex !== null) {
      event.preventDefault();
      enabledItems[nextIndex]?.focus();
    }
  };

  return (
    <div ref={ref} className="relative inline-flex">
      <button
        ref={triggerRef}
        type="button"
        id={triggerId}
        onClick={() => {
          setOpen((current) => !current);
          setOpenFocus(null);
          setMenuPosition(null);
        }}
        onKeyDown={handleTriggerKeyDown}
        className={`group inline-flex items-center transition-[background-color,color,transform] duration-150 active:translate-y-px aria-expanded:bg-[var(--color-wi-row-alt)] motion-reduce:transition-none ${
          trigger
            ? "h-7 gap-1.5 rounded-sm px-1.5 text-[var(--color-wi-text)] hover:bg-[var(--color-wi-row-alt)]"
            : "h-8 w-8 justify-center rounded-sm text-[var(--color-wi-faint)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] aria-expanded:text-[var(--color-wi-text)]"
        } ${triggerClassName}`}
        aria-label={trigger ? undefined : "More actions"}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={menuId}
      >
        {trigger ?? <MoreVertical className={`h-4 w-4 transition-transform duration-150 ${open ? "rotate-90" : ""}`} strokeWidth={2.25} aria-hidden="true" />}
      </button>
      {open && (
        createPortal(
          <div
            ref={menuRef}
            id={menuId}
            className="animate-notion-popover-in fixed z-50 w-40 max-w-[calc(100vw-0.5rem)] origin-top-right rounded-md border border-[var(--border-strong)] bg-white p-1 shadow-[0_12px_32px_rgba(0,0,0,0.14),0_1px_3px_rgba(0,0,0,0.12)] motion-reduce:animate-none"
            style={{
              top: menuPosition?.top ?? 0,
              left: menuPosition?.left ?? 0,
              visibility: menuPosition ? "visible" : "hidden",
            }}
            role="menu"
            aria-labelledby={triggerId}
            aria-orientation="vertical"
            data-state="open"
            onKeyDown={handleMenuKeyDown}
          >
            {items.map((item, index) => {
              const itemStateClass = item.danger
                ? "text-[var(--color-wi-red)] hover:bg-[var(--color-wi-danger-bg)] focus-visible:bg-[var(--color-wi-danger-bg)]"
                : "text-[var(--color-wi-text)] hover:bg-[var(--color-wi-row-alt)] focus-visible:bg-[var(--color-wi-row-alt)]";
              return (
                <button
                  key={item.label}
                  ref={(element) => { itemRefs.current[index] = element; }}
                  type="button"
                  role="menuitem"
                  disabled={item.disabled}
                  onClick={() => {
                    if (item.disabled) return;
                    item.onClick();
                    close();
                  }}
                  className={`min-h-9 w-full rounded-sm px-3 py-1.5 text-left text-sm transition-[background-color,color,transform] duration-150 focus-visible:outline-none active:translate-y-px disabled:cursor-not-allowed disabled:opacity-50 motion-reduce:transition-none ${itemStateClass}`}
                >
                  {item.label}
                </button>
              );
            })}
          </div>,
          document.body,
        )
      )}
    </div>
  );
}
