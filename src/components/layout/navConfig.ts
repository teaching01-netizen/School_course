import type { LucideIcon } from "lucide-react";
import {
  AlertOctagon,
  AlertTriangle,
  BarChart3,
  BookMarked,
  BookOpen,
  Calendar,
  CalendarDays,
  Clock,
  ClipboardCheck,
  Contact,
  Database,
  DoorOpen,
  FileText,
  FlaskConical,
  GitMerge,
  GraduationCap,
  Home,
  Inbox,
  Layers,
  LayoutDashboard,
  Mail,
  ScrollText,
  Settings,
  UserCog,
  Users,
  Workflow,
} from "lucide-react";

export interface NavItem {
  path: string;
  label: string;
  icon: LucideIcon;
  adminOnly?: boolean;
  /** Semantic badge slot; the sidebar resolves counts and colors. */
  badge?: "pending-absences-blue" | "pending-absences-amber" | "schedule-impact";
}

export interface NavSection {
  id: string;
  label: string;
  items: NavItem[];
}

export const navSections: NavSection[] = [
  {
    id: "schedule",
    label: "Schedule",
    items: [
      { path: "/", label: "Warwick Institute", icon: Home },
      { path: "/availability", label: "Availability", icon: CalendarDays },
      { path: "/slot-finder", label: "Slot Finder", icon: Clock },
    ],
  },
  {
    id: "directory",
    label: "Directory",
    items: [
      { path: "/courses", label: "Course", icon: BookOpen },
      { path: "/students", label: "Student", icon: GraduationCap },
      { path: "/teachers", label: "Teacher", icon: Users },
      { path: "/subjects", label: "Subject", icon: BookMarked },
    ],
  },
  {
    id: "absences",
    label: "Absences",
    items: [
      { path: "/absences", label: "Inbox", icon: Inbox, badge: "pending-absences-blue" },
      { path: "/absences/dashboard", label: "Dashboard", icon: LayoutDashboard, badge: "pending-absences-amber" },
      { path: "/absences/calendar", label: "Calendar", icon: Calendar },
      { path: "/admin/absence-settings", label: "Settings", icon: Settings, adminOnly: true },
    ],
  },
  {
    id: "operations",
    label: "Operations",
    items: [
      { path: "/operations/schedule-impact", label: "Schedule Impact", icon: AlertTriangle, badge: "schedule-impact" },
      { path: "/admin/operations", label: "Operations", icon: Workflow },
    ],
  },
  {
    id: "admin",
    label: "Admin & Config",
    items: [
      { path: "/classrooms", label: "Classroom", icon: DoorOpen },
      { path: "/users", label: "Users", icon: UserCog },
      { path: "/course-levels", label: "Course Levels", icon: Layers },
      { path: "/leave-policy", label: "Leave Policy", icon: FileText },
      { path: "/email-reminders", label: "Email Reminders", icon: Mail },
      { path: "/admin/sit-in-test", label: "Sit-in Test", icon: FlaskConical },
      { path: "/admin/legacy-sync", label: "Legacy Sync", icon: Database, adminOnly: true },
      { path: "/admin/legacy-sync/audit", label: "Legacy Audit", icon: ClipboardCheck, adminOnly: true },
    ],
  },
  {
    id: "audit",
    label: "Audit & CRM",
    items: [
      { path: "/reports", label: "Reports", icon: BarChart3 },
      { path: "/logs", label: "Logs", icon: ScrollText },
      { path: "/crm", label: "CRM", icon: Contact },
      { path: "/crm/conflicts", label: "Conflicts", icon: AlertOctagon },
      { path: "/crm/cross-study", label: "Cross-Study", icon: GitMerge },
    ],
  },
];

export const pageTitles: Record<string, string> = {
  "/": "Dashboard",
  "/courses": "Courses",
  "/courses/create": "New Course",
  "/students": "Students",
  "/teachers": "Teachers",
  "/teachers/create": "New Teacher",
  "/subjects": "Subjects",
  "/subjects/create": "New Subject",
  "/classrooms": "Classrooms",
  "/users": "Users",
  "/schedule": "Schedule",
  "/assign": "Assign Rooms",
  "/summary": "Summary",
  "/availability": "Availability",
  "/slot-finder": "Slot Finder",
  "/absences": "Absences",
  "/absences/dashboard": "Absence Dashboard",
  "/absences/calendar": "Operations Calendar",
  "/admin/absence-settings": "Absence Settings",
  "/reports": "Reports",
  "/logs": "Logs",
  "/teacher-dashboard": "Teacher Dashboard",
  "/course-levels": "Course Levels",
  "/admin/operations": "Operations Hub",
  "/operations/schedule-impact": "Schedule Impact",
  "/operations/session-changes": "Schedule Impact",
  "/leave-policy": "Leave Policy",
  "/email-reminders": "Email Reminders",
  "/admin/sit-in-test": "Sit-in Test",
  "/admin/legacy-sync": "Legacy Synchronization",
  "/admin/legacy-sync/audit": "Legacy Data Audit",
  "/crm": "CRM",
  "/crm/conflicts": "CRM Conflicts",
  "/crm/cross-study": "Cross-Study",
};

function matchesPath(pathname: string, itemPath: string): boolean {
  return pathname === itemPath || (itemPath !== "/" && pathname.startsWith(itemPath));
}

export function getPageTitle(pathname: string): string {
  const match = Object.entries(pageTitles).find(([path]) => matchesPath(pathname, path));
  return match ? match[1] : "Warwick Institute";
}

export function getNavTrail(pathname: string): { section?: string; page: string } {
  for (const section of navSections) {
    for (const item of section.items) {
      if (matchesPath(pathname, item.path)) {
        return { section: section.label, page: item.label };
      }
    }
  }
  return { page: getPageTitle(pathname) };
}

export function isNavActive(pathname: string, itemPath: string): boolean {
  return matchesPath(pathname, itemPath);
}