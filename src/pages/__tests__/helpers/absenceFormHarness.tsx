import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, vi } from "vitest";
import AbsenceForm from "../../AbsenceForm";
import type {
  PublicStudentLookupResponse,
  SessionsInRangeResponse,
  StudentLookupResponse,
} from "@/types";
import { renderWithProviders } from ".";
import {
  MANUAL_EMAIL_STUDENT,
  PUBLIC_FORM_CONFIG,
  PUBLIC_FORM_SESSIONS,
  parentVerification,
  publicStudentLookup,
  relativeDateKey,
  verifiedStudentProfile,
} from "../fixtures/absenceFormFixtures";

type ApiMock = ReturnType<typeof vi.fn>;
type LegacyLookupResolver = (
  wcode: string,
) => StudentLookupResponse | Promise<StudentLookupResponse>;
type LookupFixture = StudentLookupResponse | PublicStudentLookupResponse;

export type PublicFormRoutes = {
  config?: unknown;
  lookup?: LookupFixture | LegacyLookupResolver;
  sessions?: SessionsInRangeResponse | (() => SessionsInRangeResponse | Promise<SessionsInRangeResponse>);
  send?: unknown | ((init?: RequestInit) => unknown);
  verify?: unknown | ((init?: RequestInit) => unknown);
  status?: unknown;
  submission?: unknown | ((init?: RequestInit) => unknown);
};

export function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, reject, resolve };
}

export function installPublicFormRoutes(
  apiMock: ApiMock,
  routes: PublicFormRoutes = {},
) {
  let currentStudent = MANUAL_EMAIL_STUDENT;
  let currentLookup = publicStudentLookup(currentStudent);
  apiMock.mockImplementation(async (url: string, init?: RequestInit) => {
    const path = String(url);
    if (path.includes("absence-form-config")) {
      return routes.config ?? PUBLIC_FORM_CONFIG;
    }
    if (path.endsWith("/absence-self-service/lookup") || path.includes("student-lookup")) {
      const query = new URL(path, "https://absence.test").searchParams;
      const body = JSON.parse(String(init?.body ?? "{}")) as { wcode?: string };
      const wcode = query.get("wcode") ?? body.wcode ?? "";
      const configured = routes.lookup ?? MANUAL_EMAIL_STUDENT;
      const resolved = await (typeof configured === "function" ? configured(wcode) : configured);
      if ("lookup_token" in resolved) {
        currentLookup = resolved;
      } else {
        currentStudent = resolved;
        currentLookup = publicStudentLookup(currentStudent);
      }
      return currentLookup;
    }
    if (path.endsWith("/absence-self-service/me")) {
      return verifiedStudentProfile(currentStudent);
    }
    if (path.includes("/absence-self-service/sessions") || path.includes("sessions-in-range")) {
      const sessions = routes.sessions ?? PUBLIC_FORM_SESSIONS;
      return typeof sessions === "function" ? sessions() : sessions;
    }
    if (path.endsWith("/parent-verification/send")) {
      if (typeof routes.send === "function") return routes.send(init);
      return routes.send ?? parentVerification("pending", currentStudent.wcode);
    }
    if (path.endsWith("/parent-verification/verify")) {
      if (typeof routes.verify === "function") return routes.verify(init);
      return routes.verify ?? parentVerification("verified", currentStudent.wcode);
    }
    if (path.endsWith("/parent-verification/status") && init?.method === "POST") {
      if (typeof routes.status === "function") return routes.status(init);
      return routes.status ?? parentVerification("pending", currentStudent.wcode);
    }
    if (path.endsWith("/absences/batch") && init?.method === "POST") {
      if (typeof routes.submission === "function") return routes.submission(init);
      if (routes.submission !== undefined) return routes.submission;
      throw new Error(`Unmocked API call: ${path}`);
    }
    throw new Error(`Unmocked API call: ${path}`);
  });
}

export function renderPublicAbsenceForm(
  apiMock: ApiMock,
  routes: PublicFormRoutes = {},
) {
  installPublicFormRoutes(apiMock, routes);
  return renderWithProviders(<AbsenceForm />);
}

export async function startReport(
  user: ReturnType<typeof userEvent.setup>,
  wcode = MANUAL_EMAIL_STUDENT.wcode,
) {
  const input = await screen.findByRole("textbox", { name: /student id/i });
  await user.clear(input);
  await user.type(input, wcode);
  await user.click(screen.getByRole("button", { name: /^continue$/i }));
  await screen.findByRole("heading", { name: /is this you/i });
}

export async function confirmIdentity(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /yes, continue/i }));
  await screen.findByRole("heading", { name: /confirm with a parent/i });
}

export async function sendParentCode(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /^(send code|resend code)$/i }));
  const input = await findOtpInput();
  return input;
}

export async function findOtpInput() {
  return (await screen.findAllByRole("textbox", { hidden: true })).find(
    (element) => element.getAttribute("inputMode") === "numeric",
  );
}

/** Types the 6-digit code; the flow verifies automatically and advances. */
export async function enterCode(
  user: ReturnType<typeof userEvent.setup>,
  code = "123456",
) {
  const input = await findOtpInput();
  if (!input) throw new Error("OTP input was not rendered");
  await user.type(input, code);
}

export async function fillEmailIfNeeded(
  user: ReturnType<typeof userEvent.setup>,
  email = "student@example.edu",
) {
  const emailInput = screen.queryByRole("textbox", { name: /^email$/i });
  if (!emailInput) return;
  await user.type(emailInput, email);
  await user.click(screen.getByRole("button", { name: /^continue$/i }));
}

export async function completeToClasses(
  user: ReturnType<typeof userEvent.setup>,
  options: { wcode?: string; email?: string } = {},
) {
  await startReport(user, options.wcode);
  await confirmIdentity(user);
  await sendParentCode(user);
  await enterCode(user);
  await screen.findByRole("heading", { name: /where should we send updates/i });
  await fillEmailIfNeeded(user, options.email);
  await screen.findByRole("heading", { name: /which class will you miss/i });
}

export async function selectClass(
  user: ReturnType<typeof userEvent.setup>,
  label: string,
) {
  const row = screen.getByText(label).closest("label");
  if (!row) throw new Error(`No schedule row found for ${label} on the focused day`);
  await user.click(row);
}

function calendarCells(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>("[data-date-key]"));
}

function cellForDateKey(dateKey: string): HTMLElement | undefined {
  return calendarCells().find((cell) => cell.getAttribute("data-date-key") === dateKey);
}

/** Expands the schedule calendar to the full month grid. */
export async function expandCalendar(user: ReturnType<typeof userEvent.setup>) {
  const expand = screen.getByRole("button", { name: /show the whole month/i });
  await user.click(expand);
}

/** Focuses a calendar day N days from today, navigating months if needed. */
export async function openDayOffset(
  user: ReturnType<typeof userEvent.setup>,
  dayOffset: number,
) {
  const target = relativeDateKey(dayOffset);
  const expand = screen.queryByRole("button", { name: /show the whole month/i });
  if (expand) await user.click(expand);
  if (!cellForDateKey(target)) {
    const now = new Date();
    const [year, month] = target.split("-").map(Number);
    const steps = (year * 12 + (month - 1)) - (now.getFullYear() * 12 + now.getMonth());
    const label = steps < 0 ? /previous month/i : /next month/i;
    for (let i = 0; i < Math.abs(steps) + 1 && !cellForDateKey(target); i += 1) {
      await user.click(screen.getByRole("button", { name: label }));
    }
  }
  const cell = cellForDateKey(target);
  if (!cell) throw new Error(`No calendar day found for ${target}`);
  await user.click(cell);
}

export async function completeMakeUps(user: ReturnType<typeof userEvent.setup>) {
  // The fixtures carry no sit-in rules, so each make-up resolves to
  // staff-arranged and needs a single "Continue" tap.
  await screen.findByRole("heading", { name: /your make-up/i });
  const continueButton = screen.getByRole("button", { name: /^continue$/i });
  await user.click(continueButton);
}

export async function pickReason(
  user: ReturnType<typeof userEvent.setup>,
  label = "Appointment",
) {
  await screen.findByRole("heading", { name: /why will you be away/i });
  await user.click(screen.getByRole("radio", { name: label }));
}

export async function completeToReview(
  user: ReturnType<typeof userEvent.setup>,
  reason = "Appointment",
  options: { wcode?: string; email?: string; classes?: string[] } = {},
) {
  await completeToClasses(user, options);
  const classes = options.classes ?? ["Mathematics"];
  for (const label of classes) {
    await selectClass(user, label);
  }
  await user.click(screen.getByRole("button", { name: /^continue$/i }));
  await completeMakeUps(user);
  await pickReason(user, reason);
  await user.click(screen.getByRole("button", { name: /^continue$/i }));
  await screen.findByRole("heading", { name: /review your absence/i });
  await waitFor(() => {
    expect(screen.getByRole("button", { name: /submit absence/i })).toBeEnabled();
  });
}