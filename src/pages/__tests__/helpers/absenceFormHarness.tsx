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
  sessions?: SessionsInRangeResponse;
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
      return routes.sessions ?? PUBLIC_FORM_SESSIONS;
    }
    if (path.endsWith("/parent-verification/send")) {
      return parentVerification("pending", currentStudent.wcode);
    }
    if (path.endsWith("/parent-verification/verify")) {
      return parentVerification("verified", currentStudent.wcode);
    }
    if (path.endsWith("/parent-verification/status") && init?.method === "POST") {
      return parentVerification("pending", currentStudent.wcode);
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

export async function searchForStudent(
  user: ReturnType<typeof userEvent.setup>,
  wcode: string,
  method: "button" | "enter" = "button",
) {
  const input = await screen.findByRole("textbox", { name: /student id/i });
  await user.clear(input);
  await user.type(input, wcode);
  if (method === "enter") await user.keyboard("{Enter}");
  else await user.click(screen.getByRole("button", { name: /^search$/i }));
}

export async function continueThroughVerification(
  user: ReturnType<typeof userEvent.setup>,
) {
  const email = screen.queryByRole("textbox", { name: /your email address/i });
  if (email && !(email as HTMLInputElement).value) {
    await user.type(email, "student@example.edu");
  }
  await user.click(screen.getByRole("button", { name: /continue to verification/i }));
  await screen.findByRole("heading", { name: /parent verification/i });
  await user.click(screen.getByRole("button", { name: /^(send code|send new code)$/i }));
  const codeInput = (await screen.findAllByRole("textbox", { hidden: true })).find(
    (element) => element.getAttribute("inputMode") === "numeric",
  );
  if (!codeInput) throw new Error("OTP input was not rendered");
  await user.type(codeInput, "123456");
  await waitFor(() => {
    expect(screen.getByRole("heading", { name: /courses & classes/i })).toBeInTheDocument();
  });
}
