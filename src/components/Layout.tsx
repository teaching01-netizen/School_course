import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';
import { useEffect, useState } from "react";
import { ChevronDown, Menu, X } from "lucide-react";
import { apiJson } from "../api/client";
import type { AbsenceStats } from "../types";

const navGroups = [
  {
    label: "Schedule",
    items: [
      { path: '/', label: 'Warwick Institute' },
      { path: '/availability', label: 'Availability' },
      { path: '/slot-finder', label: 'Slot Finder' },
    ],
  },
  {
    label: "Directory",
    items: [
      { path: '/courses', label: 'Course' },
      { path: '/students', label: 'Student' },
      { path: '/teachers', label: 'Teacher' },
      { path: '/subjects', label: 'Subject' },
    ],
  },
  {
    label: "Admin",
    items: [
      { path: '/users', label: 'Users' },
      { path: '/classrooms', label: 'Classroom' },
    ],
  },
  {
    label: "Audit",
    items: [
      { path: '/reports', label: 'Reports' },
      { path: '/logs', label: 'Logs' },
    ],
  },
];

const adminNavItems = [{ path: '/crm', label: 'CRM' }];

const configNavItems = [
  { path: '/course-levels', label: 'Course Levels' },
  { path: '/admin/operations', label: 'Operations' },
  { path: '/leave-policy', label: 'Leave Policy' },
];

const absenceSubItems: { path: string; label: string; adminOnly?: boolean }[] = [
  { path: '/absences', label: 'Inbox' },
  { path: '/absences/dashboard', label: 'Dashboard' },
  { path: '/absences/calendar', label: 'Calendar' },
  { path: '/admin/absence-settings', label: 'Settings', adminOnly: true },
];

export default function Layout({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout } = useAuth();
  const [menuOpen, setMenuOpen] = useState(false);
  const [pendingAbsences, setPendingAbsences] = useState(0);
  const [absenceOpen, setAbsenceOpen] = useState(false);
  const [mobileAbsenceOpen, setMobileAbsenceOpen] = useState(false);

  useEffect(() => {
    if (user?.role !== "Admin") {
      setPendingAbsences(0);
      return;
    }
    let active = true;
    const refreshStats = async () => {
      try {
        const stats = await apiJson<AbsenceStats>("/api/v1/absences/stats", { method: "GET" });
        if (active) setPendingAbsences(stats.pending_count);
      } catch {
        if (active) setPendingAbsences(0);
      }
    };
    void refreshStats();
    const pollID = window.setInterval(() => void refreshStats(), 60_000);
    return () => {
      active = false;
      window.clearInterval(pollID);
    };
  }, [user?.role]);

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      navigate('/login');
    }
  };

  const renderNavLink = (item: { path: string; label: string }, onMobile?: boolean) => {
    const first = item.path === '/';
    const isActive = location.pathname === item.path || (!first && location.pathname.startsWith(item.path));

    return (
      <Link
        key={item.path}
        to={item.path}
        onClick={onMobile ? () => setMenuOpen(false) : undefined}
        aria-current={isActive ? 'page' : undefined}
        className={`${onMobile ? 'block px-3 py-2 min-h-[44px] flex items-center' : 'px-3 py-2'} text-[13px] transition-colors focus-visible:outline-offset-[-2px] ${
          first
            ? 'font-semibold'
            : isActive
              ? 'text-white underline decoration-white/70 underline-offset-[10px]'
              : 'text-slate-400 hover:text-white'
        }`}
      >
        {item.label}
      </Link>
    );
  };

  const isAbsenceActive = () =>
    location.pathname.startsWith('/absences/') ||
    location.pathname === '/absences' ||
    location.pathname === '/admin/absence-settings';

  return (
    <div className="min-h-screen bg-white flex flex-col">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-3 focus:z-50 focus:rounded-md focus:bg-white focus:px-3 focus:py-2 focus:text-[13px] focus:text-[var(--color-wi-text)] focus:shadow"
      >
        Skip to content
      </a>
      {/* Dark Top Navigation */}
      <header className="bg-[var(--color-wi-nav)] text-white shrink-0">
        <div className="max-w-[1100px] mx-auto flex items-center justify-between px-4 h-[42px]">
          <nav className="hidden md:flex items-center">
            {/* Schedule */}
            {navGroups[0].items.map((item) => renderNavLink(item))}

            <div className="w-px h-4 mx-1 bg-white/20" aria-hidden="true" />

            {/* Directory */}
            {navGroups[1].items.map((item) => renderNavLink(item))}

            <div className="w-px h-4 mx-1 bg-white/20" aria-hidden="true" />

            {/* Absences dropdown */}
            <div
              className="relative"
              onMouseEnter={() => setAbsenceOpen(true)}
              onMouseLeave={() => setAbsenceOpen(false)}
            >
              <button
                onClick={() => setAbsenceOpen((prev) => !prev)}
                className={`flex items-center gap-1 px-3 py-2 text-[13px] transition-colors focus-visible:outline-offset-[-2px] ${
                  isAbsenceActive()
                    ? 'text-white underline decoration-white/70 underline-offset-[10px]'
                    : 'text-slate-400 hover:text-white'
                }`}
                aria-expanded={absenceOpen}
                aria-haspopup="true"
              >
                Absences
                <ChevronDown className={`w-3 h-3 transition-transform ${absenceOpen ? 'rotate-180' : ''}`} />
                {user?.role === "Admin" && pendingAbsences > 0 ? (
                  <span aria-label={`${pendingAbsences} pending absences`} className="ml-0.5 inline-flex min-w-5 items-center justify-center rounded-full bg-blue-500 px-1.5 py-0.5 text-[10px] font-semibold text-white">
                    {pendingAbsences}
                  </span>
                ) : null}
              </button>
              {absenceOpen && (
                <>
                  <div className="absolute -bottom-1 left-0 right-0 h-1" />
                  <div className="absolute top-full left-0 mt-0 bg-white shadow-lg border border-gray-200 rounded-sm min-w-[160px] z-50 py-1">
                    {absenceSubItems
                      .filter((item) => !item.adminOnly || user?.role === 'Admin')
                      .map(({ adminOnly: _adminOnly, ...rest }) => {
                        const isActive = location.pathname === rest.path || (rest.path !== '/' && location.pathname.startsWith(rest.path));
                        return (
                          <Link
                            key={rest.path}
                            to={rest.path}
                            onClick={() => { setAbsenceOpen(false); }}
                            className={`block px-4 py-2 text-[13px] transition-colors ${
                              isActive
                                ? 'text-[var(--color-wi-primary)] font-medium bg-blue-50'
                                : 'text-gray-700 hover:bg-gray-50 hover:text-gray-900'
                            }`}
                          >
                            {rest.label}
                            {rest.path === '/absences' && user?.role === "Admin" && pendingAbsences > 0 ? (
                              <span className="ml-2 inline-flex min-w-5 items-center justify-center rounded-full bg-blue-500 px-1.5 py-0.5 text-[10px] font-semibold text-white">
                                {pendingAbsences}
                              </span>
                            ) : null}
                          </Link>
                        );
                      })}
                  </div>
                </>
              )}
            </div>

            <div className="w-px h-4 mx-1 bg-white/20" aria-hidden="true" />

            {/* Config */}
            {configNavItems.map((item) => renderNavLink(item))}

            <div className="w-px h-4 mx-1 bg-white/20" aria-hidden="true" />

            {/* Admin */}
            {navGroups[2].items.map((item) => renderNavLink(item))}

            <div className="w-px h-4 mx-1 bg-white/20" aria-hidden="true" />

            {/* Audit */}
            {navGroups[3].items.map((item) => renderNavLink(item))}

            {user?.role === 'Admin' && (
              <>
                <div className="w-px h-4 mx-1 bg-white/20" aria-hidden="true" />
                {adminNavItems.map((item) => renderNavLink(item))}
              </>
            )}
          </nav>

          <div className="flex items-center gap-4 text-[13px]">
            <span className="text-gray-300">Hello {user?.username ?? "—"}!</span>
            <button onClick={handleLogout} className="text-gray-300 hover:text-white">
              Log out
            </button>
          </div>
          <button
            className="md:hidden p-2 text-gray-300 hover:text-white"
            onClick={() => setMenuOpen(!menuOpen)}
            aria-label={menuOpen ? "Close menu" : "Open menu"}
          >
            {menuOpen ? <X className="w-5 h-5" /> : <Menu className="w-5 h-5" />}
          </button>
        </div>
        {menuOpen && (
          <div className="md:hidden border-t border-white/10">
            <div className="max-w-[1100px] mx-auto px-4 py-2 space-y-1">
              {navGroups.slice(0, 2).map((group) => (
                <div key={group.label}>
                  <div className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-gray-500">
                    {group.label}
                  </div>
                  {group.items.map((item) => renderNavLink(item, true))}
                </div>
              ))}

              {/* Absences mobile */}
              <div>
                <button
                  onClick={() => setMobileAbsenceOpen((prev) => !prev)}
                  className={`flex items-center gap-1 w-full px-3 py-2 min-h-[44px] text-[13px] transition-colors ${
                    isAbsenceActive()
                      ? 'text-white'
                      : 'text-slate-400'
                  }`}
                >
                  Absences
                  <ChevronDown className={`w-3 h-3 transition-transform ${mobileAbsenceOpen ? 'rotate-180' : ''}`} />
                  {user?.role === "Admin" && pendingAbsences > 0 ? (
                    <span aria-label={`${pendingAbsences} pending absences`} className="ml-1 inline-flex min-w-5 items-center justify-center rounded-full bg-blue-500 px-1.5 py-0.5 text-[10px] font-semibold text-white">
                      {pendingAbsences}
                    </span>
                  ) : null}
                </button>
                {mobileAbsenceOpen && (
                  <div className="ml-4 border-l border-white/20 pl-2 space-y-1">
                    {absenceSubItems
                      .filter((item) => !item.adminOnly || user?.role === 'Admin')
                      .map(({ adminOnly: _adminOnly, ...rest }) => renderNavLink(rest, true))}
                  </div>
                )}
              </div>

              {/* Config mobile */}
              <div>
                <div className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-gray-500">
                  Config
                </div>
                {configNavItems.map((item) => renderNavLink(item, true))}
              </div>

              {navGroups.slice(2).map((group) => (
                <div key={group.label}>
                  <div className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-gray-500">
                    {group.label}
                  </div>
                  {group.items.map((item) => renderNavLink(item, true))}
                </div>
              ))}

              {user?.role === 'Admin' && (
                <div>
                  <div className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-gray-500">
                    CRM
                  </div>
                  {adminNavItems.map((item) => renderNavLink(item, true))}
                </div>
              )}
            </div>
          </div>
        )}
      </header>

      {/* Main Content */}
      <main id="main" className="flex-1 py-4">
        <div className="max-w-[1100px] mx-auto px-4">
          {children}
        </div>
      </main>

      {/* Footer */}
      <footer className="px-4 py-3 text-[12px] text-gray-500 border-t border-gray-200">
        <div className="max-w-[1100px] mx-auto">
          © 2017 - Warwick Institute. All Rights Reserved.
        </div>
      </footer>
    </div>
  );
}
