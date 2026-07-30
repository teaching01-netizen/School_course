import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ImpactAcknowledgementModal from "./ImpactAcknowledgementModal";

it("requires an explicit acknowledgement before a schedule-impact change can be saved", async () => {
  const onConfirm = vi.fn();
  const user = userEvent.setup();
  render(<ImpactAcknowledgementModal summary={{ direct_sit_in_assignments: 1, short_notice: true }} onBack={() => {}} onConfirm={onConfirm} />);

  expect(screen.getByRole("button", { name: "Save and review impact" })).toBeDisabled();
  expect(screen.getByText("1 sit-in arrangement will be reviewed")).toBeInTheDocument();
  await user.click(screen.getByRole("checkbox"));
  await user.click(screen.getByRole("button", { name: "Save and review impact" }));
  expect(onConfirm).toHaveBeenCalledOnce();
});
