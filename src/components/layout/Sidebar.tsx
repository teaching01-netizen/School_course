import { Link, useLocation, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../../hooks/useAuth";
import { useCallback, useEffect, useRef, useState } from "react";
import { LayoutDashboard, LogOut, PanelLeftClose } from "lucide-react";
import { apiJson } from "../../api/client";
import { cachePolicies, queryKeys } from "../../query/cache";
import type { AbsenceStats } from "../../types";
import { isNavActive, navSections, type NavItem } from "./navConfig";
import WorkspaceTile from "./WorkspaceTile";

const DEFAULT_WIDTH = 252;
const MIN_WIDTH = 220;
const MAX_WIDTH = 420;
const WIDTH_KEY = "wi.sidebar.width";

interface SidebarProps {
  collapsed: boolean;
  mobileOpen: boolean;
  onCloseMobile: () => void;
}

function loadWidth(): number {
  const stored = Number(window.localStorage.getItem(WIDTH_KEY));
  if (!Number.isFinite(stored)) return DEFAULT_WIDTH;
  return Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, stored));
}

export default function Sidebar({ collapsed, mobileOpen, onCloseMobile }: SidebarProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout } = useAuth();
  const [width, setWidth] = useState(loadWidth);
  const resizeState = useRef<{ startX: number; startWidth: number } | null>(null);

  useEffect(() => {
    window.localStorage.setItem(WIDTH_KEY, String(width));
  }, [width]);

  const handleResizeStart = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    resizeState.current = { startX: e.clientX, startWidth: width };
    const onMove = (moveEvent: PointerEvent) => {
      if (!resizeState.current) return;
      const next = resizeState.current.startWidth + (moveEvent.clientX - resizeState.current.startX);
      setWidth(Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, next)));
    };
    const onUp = () => {
      resizeState.current = null;
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }, [width]);

  const isTeacher = user?.role === "Teacher";

  const statsQuery = useQuery<AbsenceStats>({
    queryKey: queryKeys.absenceStats,
    queryFn: () => apiJson<AbsenceStats>("/api/v1/absences/stats", { method: "GET" }),
    enabled: user?.role === "Admin",
    ...cachePolicies.operational,
  });
  const pendingAbsences = user?.role === "Admin" ? statsQuery.data?.pending_count ?? 0 : 0;

  const impactQuery = useQuery<{ summary?: { critical: number; need_attention: number } }>({
    queryKey: ["schedule-impact", "nav-summary"],
    queryFn: () =>
      apiJson<{ summary: { critical: number; need_attention: number } }>(
        "/api/v1/operations/schedule-impact?status=all&limit=1"
      ),
    enabled: user?.role === "Admin",
    ...cachePolicies.operational,
  });
  const criticalImpactCount = user?.role === "Admin" ? impactQuery.data?.summary?.critical ?? 0 : 0;
  const unresolvedImpactCount = user?.role === "Admin" ? impactQuery.data?.summary?.need_attention ?? 0 : 0;

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      navigate("/login");
    }
  };

  const badgeFor = (
    item: NavItem
  ): { count: number; ariaLabel: string; tone: "blue" | "amber" | "red" } | null => {
    if (item.badge === "pending-absences-blue" && pendingAbsences > 0) {
      return { count: pendingAbsences, ariaLabel: `${pendingAbsences} pending absences`, tone: "blue" };
    }
    if (item.badge === "pending-absences-amber" && pendingAbsences > 0) {
      return { count: pendingAbsences, ariaLabel: `${pendingAbsences} pending absences awaiting review`, tone: "amber" };
    }
    if (item.badge === "schedule-impact" && criticalImpactCount > 0) {
      return { count: criticalImpactCount, ariaLabel: `${criticalImpactCount} critical schedule impacts`, tone: "red" };
    }
    if (item.badge === "schedule-impact" && unresolvedImpactCount > 0) {
      return { count: unresolvedImpactCount, ariaLabel: `${unresolvedImpactCount} unresolved schedule impacts`, tone: "blue" };
    }
    return null;
  };

  const content = (
    <div className="relative flex h-full min-h-0 flex-col">
      {/* Workspace header */}
      <div className="flex h-11 shrink-0 items-center gap-2 px-2">
        <WorkspaceTile size={22} />
        <span className="min-w-0 flex-1 truncate text-[12px] font-semibold text-[var(--color-wi-text)]">
          Warwick Institute
        </span>
        <button
          type="button"
          onClick={onCloseMobile}
          className="hidden h-7 w-7 items-center justify-center rounded-sm text-[var(--color-wi-faint)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] max-md:inline-flex"
          aria-label="Close navigation"
        >
          <PanelLeftClose className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>

      {/* Nav */}
      <nav aria-label="Primary" className="notion-scrollbar min-h-0 flex-1 overflow-y-auto px-1.5 pb-2">
        {isTeacher ? (
          <SidebarRow
            item={{ path: "/teacher-dashboard", label: "Dashboard", icon: LayoutDashboard }}
            pathname={location.pathname}
            badge={null}
          />
        ) : (
          navSections.map((section) => {
            const visibleItems = section.items.filter((item) => !item.adminOnly || user?.role === "Admin");
            if (visibleItems.length === 0) return null;
            return (
              <div key={section.id} className="mb-1">
                <div className="flex h-7 items-center px-2 text-[11px] font-medium text-[var(--color-wi-faint)]">
                  {section.label}
                </div>
                <ul>
                  {visibleItems.map((item) => (
                    <SidebarRow key={item.path} item={item} pathname={location.pathname} badge={badgeFor(item)} />
                  ))}
                </ul>
              </div>
            );
          })
        )}
      </nav>

      {/* Bottom: identity + copyright */}
      <div className="shrink-0 border-t border-wi-line-soft px-2 py-1.5">
        <div className="flex items-center gap-2 rounded-sm px-1 py-1">
          <WorkspaceTile size={20} />
          <span className="min-w-0 flex-1 truncate text-[13px] font-medium text-[var(--color-wi-text)]">
            {user ? (user.full_name || user.username) : "—"}
          </span>
          <button
            type="button"
            onClick={handleLogout}
            className="inline-flex h-6 items-center gap-1 rounded-sm px-1.5 text-[12px] text-[var(--color-wi-faint)] transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)]"
          >
            <LogOut className="h-3.5 w-3.5" aria-hidden="true" />
            Log out
          </button>
        </div>
        <p className="px-1 pb-0.5 text-[11px] text-[var(--color-wi-faint)]">
          © {new Date().getFullYear()} Warwick Institute
        </p>
      </div>
    </div>
  );

  return (
    <>
      {/* Desktop sidebar */}
      <aside
        className={`relative z-30 hidden shrink-0 self-start border-r border-wi-line-soft bg-wi-bg transition-[width] duration-150 motion-reduce:transition-none md:block ${
          collapsed ? "overflow-hidden" : ""
        }`}
        style={{ width: collapsed ? 0 : width, height: "100vh", position: "sticky", top: 0 }}
        inert={collapsed}
      >
        <div className="h-full overflow-hidden" style={{ width }}>
          {content}
        </div>
        {/* Desktop resize handle */}
        {!collapsed && (
          <div
            role="separator"
            aria-orientation="vertical"
            aria-label="Resize sidebar"
            className="absolute inset-y-0 -right-[3px] z-10 w-[6px] cursor-col-resize touch-none"
            style={{ background: "transparent" }}
            onPointerDown={handleResizeStart}
            onDoubleClick={() => setWidth(DEFAULT_WIDTH)}
          >
            <div className="h-full w-full transition-colors duration-150 hover:bg-[var(--color-wi-primary)]/20" />
          </div>
        )}
      </aside>

      {/* Mobile overlay sidebar */}
      {mobileOpen && (
        <div className="fixed inset-0 z-40 md:hidden">
          <div
            className="animate-notion-backdrop-in absolute inset-0 bg-black/30 motion-reduce:animate-none"
            onClick={onCloseMobile}
            aria-hidden="true"
          />
          <aside className="absolute inset-y-0 left-0 z-10 w-[280px] max-w-[85vw] border-r border-wi-line-soft bg-wi-bg shadow-[10px_0_32px_rgba(0,0,0,0.08)]">
            <div className="h-full">{content}</div>
          </aside>
        </div>
      )}
    </>
  );
}

interface SidebarRowProps {
  item: NavItem;
  pathname: string;
  badge: { count: number; ariaLabel: string; tone: "blue" | "amber" | "red" } | null;
}

function SidebarRow({ item, pathname, badge }: SidebarRowProps) {
  const active = isNavActive(pathname, item.path);
  const Icon = item.icon;
  const toneClass =
    badge?.tone === "red"
      ? "bg-[var(--color-wi-red)] text-white"
      : badge?.tone === "amber"
        ? "bg-[var(--color-wi-amber)] text-white"
        : "bg-[var(--color-wi-primary)] text-white";

  return (
    <li>
      <div
        className={`group flex h-7 items-center rounded-sm px-2 transition-colors duration-150 ${
          active ? "bg-[var(--color-wi-selected)]" : "hover:bg-[var(--color-wi-row-alt)]"
        }`}
      >
        <Link
          to={item.path}
          aria-current={active ? "page" : undefined}
          className={`flex min-w-0 flex-1 items-center gap-2 py-1 text-[13px] transition-colors duration-150 ${
            active
              ? "font-medium text-[var(--color-wi-text)]"
              : "text-[var(--color-wi-text)] hover:text-[var(--color-wi-text)]"
          }`}
        >
          <Icon
            className={`h-[14px] w-[14px] shrink-0 transition-colors duration-150 ${
              active
                ? "text-[var(--color-wi-text)]"
                : "text-[var(--color-wi-faint)] group-hover:text-[var(--color-wi-text)]"
            }`}
            aria-hidden="true"
          />
          <span className="truncate">{item.label}</span>
        </Link>
        {badge && (
          <span
            aria-label={badge.ariaLabel}
            className={`inline-flex h-[18px] min-w-[18px] shrink-0 items-center justify-center rounded-full px-1 text-[10px] font-semibold leading-none ${toneClass}`}
          >
            {badge.count > 99 ? "99+" : badge.count}
          </span>
        )}
      </div>
    </li>
  );
}