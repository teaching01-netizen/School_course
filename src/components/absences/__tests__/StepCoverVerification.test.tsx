import { useState } from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import StepCoverVerification from "../StepCoverVerification";
import { apiJson, ApiRequestError } from "@/api/client";
import { STUDENT_SESSION_HINT_STORAGE_KEY } from "@/features/absences/constants";

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: vi.fn() };
});

const mockedApiJson = vi.mocked(apiJson);

beforeEach(() => {
  mockedApiJson.mockReset();
  window.sessionStorage.removeItem(STUDENT_SESSION_HINT_STORAGE_KEY);
});
afterEach(() => vi.useRealTimers());

type VerificationStore = {
  code: string;
  setCode: (next: string) => void;
  token: string | null;
  persistToken: (nextToken: string, nextExpiresAt?: number | null) => void;
  clearStoredToken: () => void;
};

type RenderVerificationOptions = {
  parentPhone?: string | null;
  smsParentEnabled?: boolean;
  online?: boolean;
  verification?: Partial<VerificationStore>;
  completed?: boolean;
  onSatisfied?: () => void;
  onRestart?: () => void;
  onRestored?: () => void;
};

function renderVerification(options: RenderVerificationOptions = {}) {
  const verification: VerificationStore = {
    code: "",
    setCode: vi.fn(),
    token: null,
    persistToken: vi.fn(),
    clearStoredToken: vi.fn(),
    ...options.verification,
  };
  const onSatisfied = options.onSatisfied ?? vi.fn();
  const onRestart = options.onRestart ?? vi.fn();
  const onRestored = options.onRestored ?? vi.fn();

  render(
    <StepCoverVerification
      wcode="W250389"
      lookupToken="lookup-token"
      parentPhone={options.parentPhone === undefined ? "0812345678" : options.parentPhone}
      online={options.online ?? true}
      smsParentEnabled={options.smsParentEnabled ?? true}
      verification={verification}
      completed={options.completed ?? false}
      onSatisfied={onSatisfied}
      onRestart={onRestart}
      onRestored={onRestored}
    />,
  );

  return { verification, onSatisfied, onRestart, onRestored };
}

function pendingVerification(deliveryStatus?: string) {
  return {
    token: "verification-token",
    status: "pending",
    wcode: "W250389",
    // The server always masks this value before it crosses the wire.
    parent_phone: "••••5678",
    delivery_id: "delivery-1",
    ...(deliveryStatus ? { delivery_status: deliveryStatus } : {}),
  };
}

it("does not claim an uncertain OTP delivery was sent", async () => {
  mockedApiJson.mockResolvedValueOnce({
    token: "verification-token",
    status: "pending",
    wcode: "W250389",
    parent_phone: "+66812345678",
    delivery_id: "delivery-1",
    delivery_status: "uncertain",
  });
  const user = userEvent.setup();
  renderVerification();

  await user.click(screen.getByRole("button", { name: /send code/i }));

  expect(await screen.findByText(/sms may have been sent/i)).toBeInTheDocument();
  expect(screen.queryByText(/^code sent to/i)).not.toBeInTheDocument();
});

it("polls a queued delivery until SmartSMS accepts it", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-07-14T12:00:00Z"));
  mockedApiJson
    .mockResolvedValueOnce(pendingVerification("queued"))
    .mockResolvedValueOnce(pendingVerification("accepted"));
  renderVerification();

  fireEvent.click(screen.getByRole("button", { name: /send code/i }));
  await act(async () => Promise.resolve());

  expect(screen.getByText("Sending code…")).toBeInTheDocument();
  await act(async () => vi.advanceTimersByTimeAsync(1_000));
  expect(mockedApiJson).toHaveBeenCalledWith(
    "/api/v1/absences/parent-verification/status",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ token: "verification-token" }),
    }),
  );
});

it("shows a retryable error when delivery is rejected without arming the resend cooldown", async () => {
  mockedApiJson.mockResolvedValueOnce({
    token: "verification-token",
    status: "pending",
    wcode: "W250389",
    delivery_id: "delivery-1",
    delivery_status: "failed",
  });
  const user = userEvent.setup();
  renderVerification();

  await user.click(screen.getByRole("button", { name: /send code/i }));

  expect(await screen.findByText(/couldn't send the code/i)).toBeInTheDocument();
  expect(screen.queryByText(/^code sent to/i)).not.toBeInTheDocument();
  // A failed delivery never reached the parent: the student can retry now
  // instead of waiting out the 5-minute resend cooldown.
  expect(screen.getByRole("button", { name: /^send code$/i })).toBeEnabled();
});

it("keeps the send action immediately retryable after a request failure", async () => {
  mockedApiJson
    .mockRejectedValueOnce(new TypeError("Network error"))
    .mockResolvedValueOnce(pendingVerification("accepted"));
  const user = userEvent.setup();
  renderVerification();

  await user.click(screen.getByRole("button", { name: /send code/i }));

  expect(await screen.findByText(/network error/i)).toBeInTheDocument();
  // No cooldown after a failure: the same action is enabled for an instant retry.
  expect(screen.getByRole("button", { name: /^send code$/i })).toBeEnabled();

  await user.click(screen.getByRole("button", { name: /^send code$/i }));
  expect(await screen.findByText(/^code sent to ••••5678/i)).toBeInTheDocument();
});

it("confirms an accepted OTP delivery", async () => {
  mockedApiJson.mockResolvedValueOnce(pendingVerification("accepted"));
  const user = userEvent.setup();
  renderVerification();

  await user.click(screen.getByRole("button", { name: /send code/i }));

  expect(await screen.findByText(/^code sent to ••••5678/i)).toBeInTheDocument();
  expect(screen.queryByText("Sending code…")).not.toBeInTheDocument();
});

it("reports an expired OTP without claiming delivery", async () => {
  mockedApiJson.mockResolvedValueOnce(pendingVerification("expired"));
  const user = userEvent.setup();
  renderVerification();

  await user.click(screen.getByRole("button", { name: /send code/i }));

  expect(await screen.findByText(/that code has expired/i)).toBeInTheDocument();
  expect(screen.queryByText(/^code sent to/i)).not.toBeInTheDocument();
});

it("explains a wrong code in student language, keeps it typed, and never auto-retries it", async () => {
  mockedApiJson
    .mockResolvedValueOnce(pendingVerification("accepted"))
    .mockRejectedValueOnce(new ApiRequestError("Invalid code", { status: 400 }));
  const { onSatisfied, verification } = renderVerification({
    verification: { code: "123456", token: "verification-token" },
  });

  expect(await screen.findByText(/that code isn't right/i)).toBeInTheDocument();
  // The typed code survives so one wrong digit costs one digit, not six.
  expect(verification.setCode).not.toHaveBeenCalled();
  // Give the auto-verify effect time to (wrongly) re-fire — it must not send
  // the same rejected code again.
  await new Promise((resolve) => setTimeout(resolve, 150));
  const verifyPath = "/api/v1/absences/parent-verification/verify";
  expect(mockedApiJson.mock.calls.filter(([path]) => path === verifyPath)).toHaveLength(1);
  expect(onSatisfied).not.toHaveBeenCalled();
  expect(verification.clearStoredToken).not.toHaveBeenCalled();
});

it("calls verify at most once per unchanged code while the student types and corrects", async () => {
  // Mount restore consumes one status call; each *changed* 6-digit code may
  // trigger exactly one verify request, and an unchanged code must never
  // re-fire (a rejected code used to loop until the code was edited).
  mockedApiJson
    .mockResolvedValueOnce(pendingVerification("accepted")) // saved-token restore
    .mockRejectedValueOnce(new ApiRequestError("Invalid code", { status: 400 }))
    .mockRejectedValueOnce(new ApiRequestError("Invalid code", { status: 400 }))
    .mockResolvedValueOnce({ ...pendingVerification("accepted"), status: "verified" });

  const onSatisfied = vi.fn();
  const onRestart = vi.fn();
  const onRestored = vi.fn();
  function Harness() {
    // A real store wired the way the page wires useOtp: typing through the
    // OTP input is what drives verification. Callbacks are hoisted so their
    // identities are stable, exactly as the page's useCallbacks are.
    const [code, setCode] = useState("");
    const verification: VerificationStore = {
      code,
      setCode,
      token: "verification-token",
      persistToken: vi.fn(),
      clearStoredToken: vi.fn(),
    };
    return (
      <StepCoverVerification
        wcode="W250389"
        lookupToken="lookup-token"
        parentPhone="0812345678"
        verification={verification}
        completed={false}
        onSatisfied={onSatisfied}
        onRestart={onRestart}
        onRestored={onRestored}
      />
    );
  }
  render(<Harness />);
  const user = userEvent.setup();
  const verifyPath = "/api/v1/absences/parent-verification/verify";
  const verifyCalls = () => mockedApiJson.mock.calls.filter(([path]) => path === verifyPath);
  const otp = (await screen.findAllByRole("textbox", { hidden: true })).find(
    (element) => element.getAttribute("aria-label") === "Confirmation code",
  ) as HTMLElement;

  // A complete wrong code triggers exactly one rejected request…
  await user.type(otp, "111111");
  expect(await screen.findByText(/that code isn't right/i)).toBeInTheDocument();
  await waitFor(() => expect(verifyCalls()).toHaveLength(1));
  // …and the same unchanged code never re-fires while it stays on screen.
  await new Promise((resolve) => setTimeout(resolve, 250));
  expect(verifyCalls()).toHaveLength(1);
  expect(onSatisfied).not.toHaveBeenCalled();

  // Fixing a digit changes the code: exactly one new request, then silence.
  await user.clear(otp);
  await user.type(otp, "111112");
  await waitFor(() => expect(verifyCalls()).toHaveLength(2));
  await new Promise((resolve) => setTimeout(resolve, 250));
  expect(verifyCalls()).toHaveLength(2);
  expect(onSatisfied).not.toHaveBeenCalled();

  // The corrected code verifies on its single request.
  await user.clear(otp);
  await user.type(otp, "123456");
  await waitFor(() => expect(onSatisfied).toHaveBeenCalledOnce());
  await new Promise((resolve) => setTimeout(resolve, 150));
  expect(verifyCalls()).toHaveLength(3);
});

it("automatically verifies a complete six-digit code", async () => {
  const verifiedResponse = {
    ...pendingVerification("accepted"),
    status: "verified",
  };
  mockedApiJson
    .mockResolvedValueOnce(pendingVerification("accepted"))
    .mockResolvedValueOnce(verifiedResponse);
  const { onSatisfied, verification } = renderVerification({
    verification: { code: "123456", token: "verification-token" },
  });

  await waitFor(() => expect(onSatisfied).toHaveBeenCalledOnce());
  expect(verification.clearStoredToken).toHaveBeenCalledOnce();
  expect(window.sessionStorage.getItem(STUDENT_SESSION_HINT_STORAGE_KEY)).toBe("1");

  expect(mockedApiJson).toHaveBeenCalledWith(
    "/api/v1/absences/parent-verification/verify",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ token: "verification-token", code: "123456" }),
    }),
  );
});

it("restores verification from the authenticated student session", async () => {
  window.sessionStorage.setItem(STUDENT_SESSION_HINT_STORAGE_KEY, "1");
  mockedApiJson.mockResolvedValueOnce({
    wcode: "W250389",
    display_name: "Alex",
    email_on_file: true,
    subjects: [],
  });
  const { onRestored } = renderVerification();

  await waitFor(() => expect(onRestored).toHaveBeenCalledOnce());
  expect(mockedApiJson).toHaveBeenCalledWith(
    "/api/v1/absence-self-service/me",
    expect.objectContaining({ method: "GET" }),
  );
});

it("retries the same code after a retryable verification error only when the student asks", async () => {
  const verifiedResponse = {
    ...pendingVerification("accepted"),
    status: "verified",
  };
  mockedApiJson
    .mockResolvedValueOnce(pendingVerification("accepted"))
    .mockRejectedValueOnce(new ApiRequestError("Verification service unavailable", { status: 503 }))
    .mockResolvedValueOnce(verifiedResponse);
  const user = userEvent.setup();
  const { onSatisfied, verification } = renderVerification({
    verification: { code: "123456", token: "verification-token" },
  });

  expect(await screen.findByText("Verification service unavailable")).toBeInTheDocument();
  expect(verification.setCode).not.toHaveBeenCalled();

  await user.click(screen.getByRole("button", { name: /retry verification/i }));

  await waitFor(() => expect(onSatisfied).toHaveBeenCalledOnce());
  const verifyCalls = mockedApiJson.mock.calls.filter(([path]) => path === "/api/v1/absences/parent-verification/verify");
  expect(verifyCalls).toHaveLength(2);
  expect(verifyCalls[0]?.[1]).toEqual(expect.objectContaining({
    body: JSON.stringify({ token: "verification-token", code: "123456" }),
  }));
  expect(verifyCalls[1]?.[1]).toEqual(expect.objectContaining({
    body: JSON.stringify({ token: "verification-token", code: "123456" }),
  }));
});

it.each([400, 410])("restarts when saved-token restore returns %i", async (status) => {
  mockedApiJson.mockRejectedValueOnce(new ApiRequestError("Saved verification is invalid", { status }));
  const { onRestart } = renderVerification({
    verification: { token: "stale-token" },
  });

  await waitFor(() => expect(onRestart).toHaveBeenCalledOnce());

  expect(screen.queryByRole("button", { name: /retry verification check/i })).not.toBeInTheDocument();
});

it("retries a saved-token restore after a transient failure", async () => {
  mockedApiJson
    .mockRejectedValueOnce(new TypeError("Network unavailable"))
    .mockResolvedValueOnce({
      ...pendingVerification("accepted"),
      token: "saved-token",
      status: "verified",
    });
  const onRestored = vi.fn();
  const user = userEvent.setup();
  renderVerification({
    verification: { token: "saved-token" },
    onRestored,
  });

  expect(await screen.findByText(/could not validate saved verification/i)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: /retry verification check/i }));

  await waitFor(() => expect(onRestored).toHaveBeenCalledOnce());
  expect(mockedApiJson).toHaveBeenCalledTimes(2);
});


it("disables network verification while offline", () => {
  renderVerification({ online: false });

  expect(screen.getByRole("status")).toHaveTextContent(/offline/i);
  expect(screen.getByRole("button", { name: /send code/i })).toBeDisabled();
});
it("does not offer a way to continue without verifying", () => {
  renderVerification({ parentPhone: null });

  expect(screen.queryByRole("button", { name: /continue without verifying/i })).not.toBeInTheDocument();
});

it.each([
  {
    label: "SMS on, phone present",
    smsParentEnabled: true,
    parentPhone: "0812345678",
    sendVisible: true,
    enrollVisible: false,
    bypassVisible: false,
    alertText: null,
  },
  {
    // No phone on file is no longer a dead end: the enrollment input is
    // offered, and the send action appears only once a valid number has been
    // entered (inside the confirm-number panel).
    label: "SMS on, phone missing offers enrollment",
    smsParentEnabled: true,
    parentPhone: null,
    sendVisible: false,
    enrollVisible: true,
    bypassVisible: false,
    alertText: null,
  },
  {
    label: "SMS off, phone present",
    smsParentEnabled: false,
    parentPhone: "0812345678",
    sendVisible: false,
    enrollVisible: false,
    bypassVisible: false,
    alertText: /verification codes are currently unavailable.*contact admin/i,
  },
  {
    label: "SMS off, phone missing",
    smsParentEnabled: false,
    parentPhone: null,
    sendVisible: false,
    enrollVisible: false,
    bypassVisible: false,
    alertText: /verification codes are currently unavailable.*contact admin/i,
  },
])("enforces the OTP policy matrix: $label", ({
  smsParentEnabled,
  parentPhone,
  sendVisible,
  enrollVisible,
  bypassVisible,
  alertText,
}) => {
  renderVerification({ smsParentEnabled, parentPhone });

  const sendButton = screen.queryByRole("button", { name: /^send code$/i });
  const bypassButton = screen.queryByRole("button", { name: /continue without verifying/i });
  const enrollInput = screen.queryByLabelText(/parent's phone number/i);
  expect(Boolean(sendButton)).toBe(sendVisible);
  expect(Boolean(bypassButton)).toBe(bypassVisible);
  expect(Boolean(enrollInput)).toBe(enrollVisible);

  const alert = screen.queryByRole("alert");
  if (alertText) {
    expect(alert).toHaveTextContent(alertText);
  } else {
    expect(alert).not.toBeInTheDocument();
  }
});

it("enrolls a client-provided parent phone when none is on file", async () => {
  mockedApiJson.mockResolvedValueOnce({
    token: "verification-token",
    status: "pending",
    wcode: "W250389",
    parent_phone: "••••8888",
    delivery_status: "accepted",
  });
  const user = userEvent.setup();
  renderVerification({ parentPhone: null });

  const input = screen.getByLabelText(/parent's phone number/i);
  // No send action until a valid number is entered and confirmed.
  expect(screen.queryByRole("button", { name: /^send code$/i })).not.toBeInTheDocument();

  await user.type(input, "08");
  expect(screen.queryByRole("button", { name: /^send code$/i })).not.toBeInTheDocument();
  await user.type(input, "99998888");

  // The number is echoed back for confirmation before any code goes out.
  expect(screen.getByText(/confirm this number/i)).toBeInTheDocument();
  expect(screen.getByText("• ••• ••• •88")).toBeInTheDocument();
  expect(screen.queryByText("0899998888")).not.toBeInTheDocument();
  const confirmSend = screen.getByRole("button", { name: /^send code$/i });
  expect(confirmSend).toBeEnabled();
  await user.click(confirmSend);

  await waitFor(() =>
    expect(mockedApiJson).toHaveBeenCalledWith(
      "/api/v1/absences/parent-verification/send",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ lookup_token: "lookup-token", parent_phone: "0899998888" }),
      }),
    ),
  );
  expect(await screen.findByText(/^code sent to ••••8888/i)).toBeInTheDocument();
});

it("lets the student change the number from the confirmation step", async () => {
  const user = userEvent.setup();
  renderVerification({ parentPhone: null });

  const input = screen.getByLabelText(/parent's phone number/i);
  await user.type(input, "0899998888");
  expect(screen.getByText(/confirm this number/i)).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: /change number/i }));

  const inputAgain = screen.getByLabelText(/parent's phone number/i);
  expect(inputAgain).toHaveValue("");
  expect(screen.queryByText(/confirm this number/i)).not.toBeInTheDocument();
});

it("keeps typing working while the confirmation panel is shown", async () => {
  const user = userEvent.setup();
  renderVerification({ parentPhone: null });

  const input = screen.getByLabelText(/parent's phone number/i);
  await user.type(input, "0899998888");

  // The panel appears once the number is valid but never steals the field.
  expect(screen.getByText(/confirm this number/i)).toBeInTheDocument();
  expect(input).toHaveValue("0899998888");
  await user.type(input, "9");
  expect(input).toHaveValue("08999988889");
});
