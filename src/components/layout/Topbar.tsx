import { useEffect } from "react";
import { useLocation } from "react-router-dom";
import { Menu, PanelLeft, X } from "lucide-react";
import { getNavTrail, getPageTitle } from "./navConfig";

interface TopbarProps {
  collapsed: boolean;
  mobileOpen: boolean;
  onToggleCollapse: () => void;
  onOpenMobile: () => void;
}

export default function Topbar({ collapsed, mobileOpen, onToggleCollapse, onOpenMobile }: TopbarProps) {
  const location = useLocation();
  const title = getPageTitle(location.pathname);
  const trail = getNavTrail(location.pathname);

  useEffect(() => {
    document.title = title === "Warwick Institute" ? "Warwick Institute" : `${title} — Warwick Institute`;
  }, [title]);

  return (
    <header className="sticky top-0 z-20 flex h-11 shrink-0 items-center gap-1 border-b border-b-[var(--color-wi-line)] bg-white px-3">
      {/* Mobile menu toggle */}
      <button
        type="button"
        onClick={onOpenMobile}
        className="inline-flex h-8 w-8 items-center justify-center rounded-sm text-[var(--color-wi-faint)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] md:hidden"
        aria-label={mobileOpen ? "Close navigation" : "Open navigation"}
      >
        {mobileOpen ? (
          <X className="h-4 w-4" aria-hidden="true" />
        ) : (
          <Menu className="h-4 w-4" aria-hidden="true" />
        )}
      </button>

      {/* Desktop sidebar collapse toggle */}
      <button
        type="button"
        onClick={onToggleCollapse}
        className="hidden h-8 w-8 items-center justify-center rounded-sm text-[var(--color-wi-faint)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] md:inline-flex"
        aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
      >
        <PanelLeft
          className={`h-4 w-4 transition-transform duration-200 motion-reduce:transition-none ${
            collapsed ? "-rotate-180" : ""
          }`}
          aria-hidden="true"
        />
      </button>

      {/* Breadcrumb: section / page */}
      <div className="flex min-w-0 items-center gap-1.5 pl-1">
        {trail.section && (
          <span className="hidden truncate text-[13px] text-[var(--color-wi-faint)] sm:block">{trail.section}</span>
        )}
        {trail.section && (
          <span className="hidden text-[13px] text-[var(--color-wi-faint)] sm:block" aria-hidden="true">
            /
          </span>
        )}
        <div className="truncate text-[13px] font-medium text-[var(--color-wi-text)]">{title}</div>
      </div>
    </header>
  );
}