import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StudentLookupResponse } from "@/types";
import {
  CRM_EMAIL_STUDENT,
  MANUAL_EMAIL_STUDENT,
  PUBLIC_FORM_CONFIG,
  SECOND_STUDENT,
  SYSTEM_EMAIL_STUDENT,
  sessionsWithAlreadyAbsent,
} from "./fixtures/absenceFormFixtures";
import {
  continueThroughVerification,
  deferred,
  renderPublicAbsenceForm,
  searchForStudent,
} from "./helpers/absenceFormHarness";

const mockApiJson = vi.hoisted(() => vi.fn());

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: mockApiJson };
});

vi.mock("react-router-dom", () => ({
  useNavigate: () => vi.fn(),
  useLocation: () => ({ pathname: "/absence" }),
}));

const STUDENT_RESUME_STORAGE_KEY = "warwick-absence-form-student-v1";

describe("AbsenceForm Student step", () => {
  beforeEach(() => {
    mockApiJson.mockReset();
    window.localStorage?.clear();
    window.sessionStorage?.clear();
  });

  it("rejects a blank W-Code without sending a lookup request", async () => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson);

    await user.click(await screen.findByRole("button", { name: /^search$/i }));

    expect(screen.getByText("Enter your Student ID (W-Code).", { selector: '[role="alert"]' })).toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("student-lookup"))).toBe(false);
  });

  it("only exposes W-Code lookup and rejects nickname input without sending a lookup", async () => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson);

    const input = await screen.findByRole("textbox", { name: "Student ID (W-Code)" });
    expect(input).toHaveAttribute("placeholder", "e.g. W250389");
    expect(screen.queryByText(/nickname/i)).not.toBeInTheDocument();

    await user.type(input, "Johnny");
    await user.click(screen.getByRole("button", { name: /^search$/i }));

    expect(screen.getByText("Enter your Student ID (W-Code).", { selector: '[role="alert"]' })).toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url]) => String(url).includes("student-lookup"))).toBe(false);
  });

  it.each(["  w250389  ", "W250389"])(
    "normalizes and searches %j using Enter",
    async (input) => {
      const user = userEvent.setup();
      renderPublicAbsenceForm(mockApiJson);

      await searchForStudent(user, input, "enter");

      expect(await screen.findByText(MANUAL_EMAIL_STUDENT.full_name)).toBeInTheDocument();
      expect(mockApiJson).toHaveBeenCalledWith(
        "/api/v1/absences/student-lookup?wcode=W250389",
        expect.objectContaining({ method: "GET" }),
      );
    },
  );

  it.each([
    ["CRM", CRM_EMAIL_STUDENT, "alex.crm@example.edu"],
    ["System", SYSTEM_EMAIL_STUDENT, "alex.system@example.edu"],
  ])("shows the %s email as authoritative", async (source, student, email) => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson, { lookup: student });

    await searchForStudent(user, student.wcode);

    expect(await screen.findByText(email)).toBeInTheDocument();
    expect(screen.getByText(source)).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: /your email address/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /continue to verification/i })).toBeEnabled();
  });

  it("hides SMS sending and does not expose a verification bypass when parent SMS is disabled", async () => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson, {
      config: {
        ...PUBLIC_FORM_CONFIG,
        notifications: {
          ...PUBLIC_FORM_CONFIG.notifications,
          sms_parent_enabled: false,
          allow_submit_without_otp: true,
        },
      },
      lookup: CRM_EMAIL_STUDENT,
    });
    await searchForStudent(user, CRM_EMAIL_STUDENT.wcode);
    await screen.findByText(CRM_EMAIL_STUDENT.full_name);
    await user.click(screen.getByRole("button", { name: /continue to verification/i }));

    expect(await screen.findByRole("heading", { name: /parent verification/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^send code$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /continue without verifying/i })).not.toBeInTheDocument();
  });

  it("requires a manual email when no authoritative source is returned", async () => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson);
    await searchForStudent(user, MANUAL_EMAIL_STUDENT.wcode);

    const email = await screen.findByRole("textbox", { name: /your email address/i });
    const continueButton = screen.getByRole("button", { name: /continue to verification/i });
    expect(email).toHaveAttribute("type", "email");
    expect(continueButton).toBeDisabled();

    await user.type(email, "student@example.edu");

    expect(continueButton).toBeEnabled();
    await waitFor(() => {
      expect(JSON.parse(window.sessionStorage.getItem(STUDENT_RESUME_STORAGE_KEY) ?? "{}")).toEqual({
        wcode: MANUAL_EMAIL_STUDENT.wcode,
        collectedEmail: "student@example.edu",
      });
    });
  });

  it("keeps Continue disabled when the manual email is invalid", async () => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson);
    await searchForStudent(user, MANUAL_EMAIL_STUDENT.wcode);

    const email = await screen.findByRole("textbox", { name: /your email address/i });
    await user.type(email, "not-an-email");

    expect(email).toBeInvalid();
    expect(screen.getByRole("button", { name: /continue to verification/i })).toBeDisabled();
  });

  it("keeps the newest student when an older lookup resolves last", async () => {
    const first = deferred<StudentLookupResponse>();
    const second = deferred<StudentLookupResponse>();
    renderPublicAbsenceForm(mockApiJson, {
      lookup: (wcode) => wcode === MANUAL_EMAIL_STUDENT.wcode ? first.promise : second.promise,
    });

    const input = await screen.findByRole("textbox", { name: /student id/i });
    fireEvent.change(input, { target: { value: MANUAL_EMAIL_STUDENT.wcode } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      `/api/v1/absences/student-lookup?wcode=${MANUAL_EMAIL_STUDENT.wcode}`,
      expect.objectContaining({ method: "GET" }),
    ));
    fireEvent.change(input, { target: { value: SECOND_STUDENT.wcode } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(mockApiJson).toHaveBeenCalledWith(
      `/api/v1/absences/student-lookup?wcode=${SECOND_STUDENT.wcode}`,
      expect.objectContaining({ method: "GET" }),
    ));

    await act(async () => {
      second.resolve(SECOND_STUDENT);
      await second.promise;
    });
    expect(await screen.findByText(SECOND_STUDENT.full_name)).toBeInTheDocument();

    await act(async () => {
      first.resolve(MANUAL_EMAIL_STUDENT);
      await first.promise;
    });

    expect(screen.getByText(SECOND_STUDENT.full_name)).toBeInTheDocument();
    expect(screen.queryByText(MANUAL_EMAIL_STUDENT.full_name)).not.toBeInTheDocument();
  });

  it("stops the loading state when a blank search supersedes a pending lookup", async () => {
    const pending = deferred<StudentLookupResponse>();
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson, { lookup: () => pending.promise });

    const input = await screen.findByRole("textbox", { name: /student id/i });
    await user.type(input, MANUAL_EMAIL_STUDENT.wcode);
    await user.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "..." })).toBeDisabled();

    await user.clear(input);
    await user.keyboard("{Enter}");

    expect(screen.getByText("Enter your Student ID (W-Code).", { selector: '[role="alert"]' })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^search$/i })).toBeEnabled();
  });

  it("restores the W-Code and collected email while refetching the student", async () => {
    window.sessionStorage.setItem(STUDENT_RESUME_STORAGE_KEY, JSON.stringify({
      wcode: "w250389",
      collectedEmail: "resumed@example.edu",
    }));
    renderPublicAbsenceForm(mockApiJson);

    expect(await screen.findByText(MANUAL_EMAIL_STUDENT.full_name)).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /student id/i })).toHaveValue("W250389");
    expect(screen.getByRole("textbox", { name: /your email address/i })).toHaveValue("resumed@example.edu");
    expect(mockApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/student-lookup?wcode=W250389",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("keeps Student B authoritative and clears Student A selections and reason", async () => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson, {
      lookup: (wcode) => wcode === SECOND_STUDENT.wcode ? SECOND_STUDENT : MANUAL_EMAIL_STUDENT,
    });
    await searchForStudent(user, MANUAL_EMAIL_STUDENT.wcode);
    await screen.findByText(MANUAL_EMAIL_STUDENT.full_name);
    await continueThroughVerification(user);
    await user.click(screen.getByRole("textbox", { name: /reason for absence/i }));
    await user.type(screen.getByRole("textbox", { name: /reason for absence/i }), "Student A private reason");
    await user.click(screen.getByRole("checkbox", { name: /mathematics/i }));
    await user.click(screen.getByRole("button", { name: /student - completed/i }));

    await searchForStudent(user, SECOND_STUDENT.wcode);
    expect(await screen.findByText(SECOND_STUDENT.full_name)).toBeInTheDocument();
    await continueThroughVerification(user);

    expect(screen.getByRole("textbox", { name: /reason for absence/i })).toHaveValue("");
    expect(screen.getByRole("checkbox", { name: /physics/i })).not.toBeChecked();
  });

  it("disables an already-absent session so it cannot enter review or submission", async () => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson, { sessions: sessionsWithAlreadyAbsent() });
    await searchForStudent(user, MANUAL_EMAIL_STUDENT.wcode);
    await screen.findByText(MANUAL_EMAIL_STUDENT.full_name);
    await continueThroughVerification(user);
    await user.click(screen.getByRole("checkbox", { name: /mathematics/i }));

    const session = (await screen.findAllByRole("checkbox")).find(
      (checkbox) => checkbox.id.startsWith("session-"),
    );
    expect(session).toBeDefined();
    expect(session).toBeDisabled();
  });

  it("keeps an already-absent session out of the selection and review flow", async () => {
    const user = userEvent.setup();
    renderPublicAbsenceForm(mockApiJson, { sessions: sessionsWithAlreadyAbsent() });
    await searchForStudent(user, MANUAL_EMAIL_STUDENT.wcode);
    await screen.findByText(MANUAL_EMAIL_STUDENT.full_name);
    await continueThroughVerification(user);
    await user.click(screen.getByRole("checkbox", { name: /mathematics/i }));

    const session = (await screen.findAllByRole("checkbox")).find(
      (checkbox) => checkbox.id.startsWith("session-"),
    );
    if (!session) throw new Error("Expected the already-absent session row");
    await user.click(session);

    expect(session).not.toBeChecked();
    await user.type(screen.getByRole("textbox", { name: /reason for absence/i }), "Already reported");
    await user.click(screen.getByRole("button", { name: /review absence/i }));
    expect(screen.getByText("Select at least one class you will miss.", { selector: '[role="alert"]' })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /review your absence/i })).not.toBeInTheDocument();
    expect(mockApiJson.mock.calls.some(([url]) => String(url).endsWith("/absences/batch"))).toBe(false);
  });
});
