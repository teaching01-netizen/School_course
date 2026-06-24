import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import StepCoverVerification from "../StepCoverVerification";
import { apiJson } from "@/api/client";

vi.mock("@/api/client", async () => {
  const actual = await vi.importActual<typeof import("@/api/client")>("@/api/client");
  return { ...actual, apiJson: vi.fn() };
});

const mockedApiJson = vi.mocked(apiJson);

beforeEach(() => mockedApiJson.mockReset());

function renderVerification() {
  render(
    <StepCoverVerification
      wcode="W250389"
      parentPhone="0812345678"
      allowSubmitWithoutOtp={false}
      verification={{
        code: "",
        setCode: vi.fn(),
        token: null,
        persistToken: vi.fn(),
        clearStoredToken: vi.fn(),
      }}
      completed={false}
      onSatisfied={vi.fn()}
      onRestart={vi.fn()}
      onRestored={vi.fn()}
    />,
  );
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
  mockedApiJson
    .mockResolvedValueOnce({
      token: "verification-token",
      status: "pending",
      wcode: "W250389",
      delivery_id: "delivery-1",
      delivery_status: "queued",
    })
    .mockResolvedValueOnce({
      token: "verification-token",
      status: "pending",
      wcode: "W250389",
      delivery_id: "delivery-1",
      delivery_status: "accepted",
    });
  const user = userEvent.setup();
  renderVerification();

  await user.click(screen.getByRole("button", { name: /send code/i }));

  expect(await screen.findByText("Sending code…")).toBeInTheDocument();
  expect(await screen.findByText(/^code sent to/i, {}, { timeout: 2500 })).toBeInTheDocument();
  expect(mockedApiJson).toHaveBeenCalledWith(
    "/api/v1/absences/parent-verification/verification-token",
    expect.objectContaining({ method: "GET" }),
  );
});

it("shows a retryable error when delivery is rejected", async () => {
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
});
