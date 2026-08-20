import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import Sidebar from "./Sidebar";
import Topbar from "./Topbar";
import { useMainInert } from "../../hooks/useMainInert";

const COLLAPSED_KEY = "wi.sidebar.collapsed";

function safeGetCollapsed(): boolean {
  try {
    if (typeof window === "undefined") return false;
    return window.localStorage.getItem(COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

export default function AppShell({ children }: { children: React.ReactNode }) {
  const [collapsed, setCollapsed] = useState<boolean>(() => safeGetCollapsed());
  const [mobileOpen, setMobileOpen] = useState(false);
  const location = useLocation();
  const { add, remove } = useMainInert();

  useEffect(() => {
    try {
      window.localStorage.setItem(COLLAPSED_KEY, collapsed ? "1" : "0");
    } catch {}
  }, [collapsed]);

  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname]);

  useEffect(() => {
    if (!mobileOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMobileOpen(false);
    };
    document.addEventListener("keydown", onKey);
    add("app-shell-mobile-nav");
    return () => {
      document.removeEventListener("keydown", onKey);
      remove("app-shell-mobile-nav");
    };
  }, [mobileOpen, add, remove]);

  return (
    <div className="flex min-h-screen bg-white">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-3 focus:z-50 focus:rounded-md focus:bg-white focus:px-3 focus:py-2 focus:text-[13px] focus:text-[var(--color-wi-text)] focus:shadow"
      >
        Skip to content
      </a>

      <Sidebar collapsed={collapsed} mobileOpen={mobileOpen} onCloseMobile={() => setMobileOpen(false)} />

      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar
          collapsed={collapsed}
          mobileOpen={mobileOpen}
          onToggleCollapse={() => setCollapsed((v) => !v)}
          onOpenMobile={() => setMobileOpen(true)}
        />
        <main id="main" className="flex-1">
          <div
            className="mx-auto w-full max-w-[1080px] px-6 py-6 md:px-8"
            style={{
              paddingLeft: "max(1.5rem, env(safe-area-inset-left))",
              paddingRight: "max(1.5rem, env(safe-area-inset-right))",
            }}
          >
            {children}
          </div>
        </main>
      </div>
    </div>
  );
}
