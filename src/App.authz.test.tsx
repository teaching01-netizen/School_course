import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";

const mockUseAuth = vi.hoisted(() => vi.fn());

vi.mock("./hooks/useAuth", () => ({
  AuthProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  useAuth: mockUseAuth,
}));

vi.mock("./query/useUserScopedQueryCache", () => ({
  useUserScopedQueryCache: () => true,
}));

vi.mock("./realtime/RealtimeProvider", () => ({
  RealtimeProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("./realtime/queryBridge", () => ({
  invalidateRealtimeBackedQueries: vi.fn(),
  RealtimeQueryBridge: () => null,
}));

vi.mock("./components/Layout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("./pages/Login", () => ({
  default: () => <div>Login Page</div>,
}));

vi.mock("./pages/AbsenceForm", () => ({
  default: () => <div>Absence Form Page</div>,
}));

vi.mock("./pages/Home", () => ({
  default: () => <div>Home Page</div>,
}));

vi.mock("./pages/Courses", () => ({
  default: () => <div>Courses Page</div>,
}));

vi.mock("./pages/TeacherDashboard", () => ({
  default: () => <div>Teacher Dashboard Page</div>,
}));

vi.mock("./pages/TeacherAbsenceDetail", () => ({
  default: () => <div>Teacher Absence Detail Page</div>,
}));

import App from "./App";

function setPath(path: string) {
  window.history.pushState({}, "", path);
}

function setAnonymous() {
  mockUseAuth.mockReturnValue({ user: null, loading: false, login: vi.fn(), logout: vi.fn(), refresh: vi.fn() });
}

function setTeacher() {
  mockUseAuth.mockReturnValue({
    user: { id: "teacher-1", username: "teacher", role: "Teacher" },
    loading: false,
    login: vi.fn(),
    logout: vi.fn(),
    refresh: vi.fn(),
  });
}

function setAdmin() {
  mockUseAuth.mockReturnValue({
    user: { id: "admin-1", username: "admin", role: "Admin" },
    loading: false,
    login: vi.fn(),
    logout: vi.fn(),
    refresh: vi.fn(),
  });
}

describe("App route authz", () => {
  beforeEach(() => {
    mockUseAuth.mockReset();
    setAnonymous();
    setPath("/");
  });

  it("sends anonymous users to login from authenticated pages", async () => {
    setPath("/courses");
    render(<App />);

    expect(await screen.findByText("Login Page")).toBeInTheDocument();
  });

  it("sends teachers on admin pages to the teacher dashboard", async () => {
    setTeacher();
    setPath("/courses");
    render(<App />);

    expect(await screen.findByText("Teacher Dashboard Page")).toBeInTheDocument();
    expect(screen.queryByText("Courses Page")).not.toBeInTheDocument();
  });

  it("allows teachers to open the teacher absence detail route", async () => {
    setTeacher();
    setPath("/teacher-dashboard/absences/abs-1");
    render(<App />);

    expect(await screen.findByText("Teacher Absence Detail Page")).toBeInTheDocument();
  });

  it("keeps admin pages open for administrators", async () => {
    setAdmin();
    setPath("/courses");
    render(<App />);

    expect(await screen.findByText("Courses Page")).toBeInTheDocument();
  });

  it("keeps the public absence form open without authentication", async () => {
    setAnonymous();
    setPath("/absence");
    render(<App />);

    expect(await screen.findByText("Absence Form Page")).toBeInTheDocument();
  });
});
