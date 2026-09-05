import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ReasonScreen from "../public-form/ReasonScreen";
import SuccessScreen from "../public-form/SuccessScreen";
import MakeUpScreen, { type MakeUpPlanView } from "../public-form/MakeUpScreen";

const CATEGORIES = [
  { value: "appointment", label: "Appointment" },
  { value: "travel", label: "Travel" },
];

function renderReason(selected: string | null, onSelect = vi.fn()) {
  render(
    <ReasonScreen
      categories={CATEGORIES}
      selected={selected}
      detail=""
      requireDetailFor={() => false}
      allowFreeText={false}
      required
      onSelect={onSelect}
      onDetailChange={vi.fn()}
    />,
  );
  return onSelect;
}

describe("ReasonScreen sticky radios (a11y F6)", () => {
  it("re-activating the selected reason keeps it instead of toggling off", () => {
    const onSelect = renderReason("appointment");
    fireEvent.click(screen.getByRole("radio", { name: "Appointment" }));
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith("appointment");
  });

  it("selecting a different reason moves selection to it", () => {
    const onSelect = renderReason("appointment");
    fireEvent.click(screen.getByRole("radio", { name: "Travel" }));
    expect(onSelect).toHaveBeenCalledWith("travel");
  });
});

describe("ReasonScreen group error association (a11y F6)", () => {
  it("associates the group with the error message when present", () => {
    const { rerender } = render(
      <ReasonScreen
        categories={CATEGORIES}
        selected={null}
        detail=""
        requireDetailFor={() => false}
        allowFreeText={false}
        required
        onSelect={vi.fn()}
        onDetailChange={vi.fn()}
      />,
    );
    rerender(
      <ReasonScreen
        categories={CATEGORIES}
        selected={null}
        detail=""
        requireDetailFor={() => false}
        allowFreeText={false}
        required
        onSelect={vi.fn()}
        onDetailChange={vi.fn()}
        error="Choose a reason or tell us why you'll be away."
      />,
    );
    const group = screen.getByRole("radiogroup", { name: "Reason" });
    const error = screen.getByRole("alert");
    expect(group).toHaveAttribute("aria-invalid", "true");
    expect(group).toHaveAttribute("aria-describedby", error.id);
    expect(error.id).toBe("absence-reason-error");
  });

  it("marks the group valid when no error is present", () => {
    render(
      <ReasonScreen
        categories={CATEGORIES}
        selected={null}
        detail=""
        requireDetailFor={() => false}
        allowFreeText={false}
        required
        onSelect={vi.fn()}
        onDetailChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("radiogroup", { name: "Reason" })).toHaveAttribute(
      "aria-invalid",
      "false",
    );
  });
});

describe("SuccessScreen receipt focus (a11y F8)", () => {
  it("exposes the receipt heading as a programmatic focus target", () => {
    render(<SuccessScreen groups={[]} reference="ABC12345" onDone={vi.fn()} />);
    const heading = screen.getByRole("heading", { name: /absence report submitted/i });
    expect(heading).toHaveAttribute("id", "success-heading");
    expect(heading).toHaveAttribute("tabindex", "-1");
  });
});

const SHEET_PLANS: MakeUpPlanView[] = [
  {
    sessionKey: "miss-1",
    label: "Mathematics",
    when: "Tue 12 May",
    method: "physical",
    options: [
      { value: "a", name: "Chemistry Make-up", date: "Wed 13 May", time: "16:30–18:30" },
      { value: "b", name: "Chemistry Make-up", date: "Thu 14 May", time: "19:00–21:00" },
    ],
    selectedValue: "",
    hasMoreTimes: false,
    needsAttention: false,
  },
];

function renderSheet() {
  render(
    <MakeUpScreen
      plans={SHEET_PLANS}
      focusSessionKey={null}
      onUseTime={vi.fn()}
      onSeeMoreTimes={vi.fn()}
    />,
  );
}

describe("MakeUpScreen sheet single-select (a11y F4)", () => {
  it("exposes the time list as a radiogroup of radios, not toggle buttons", async () => {
    const user = (await import("@testing-library/user-event")).default.setup();
    renderSheet();
    await user.click(screen.getByRole("button", { name: /choose a time/i }));
    const dialog = await screen.findByRole("dialog", { name: /choose a make-up time/i });
    const group = within(dialog).getByRole("radiogroup");
    expect(group).toBeInTheDocument();
    const options = within(dialog).getAllByRole("radio");
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveAttribute("aria-checked", "false");
    await user.click(options[1]);
    expect(options[1]).toHaveAttribute("aria-checked", "true");
    expect(options[0]).toHaveAttribute("aria-checked", "false");
  });
});

describe("AbsenceActionBar blocked-Continue hints (F3)", () => {
  it("announces the hint alongside the disabled Continue via aria-describedby", async () => {
    const { default: AbsenceActionBar } = await import("../public-form/AbsenceActionBar");
    const { default: AbsenceForm } = await import("@/pages/AbsenceForm");
    expect(AbsenceForm).toBeTruthy();
    render(<AbsenceActionBar canProceed={false} onBack={() => {}} onPrimary={() => {}} primaryLabel="Continue" hint="Choose at least one class day to continue." />);
    const primary = screen.getByRole("button", { name: /^continue$/i });
    expect(primary).toBeDisabled();
    expect(primary).toHaveAttribute("aria-describedby", "absence-action-hint");
    expect(screen.getByText(/choose at least one class day/i)).toBeInTheDocument();
  });
});

describe("MakeUpScreen sheet dismiss keeps the pending choice (F4 model)", () => {
  it("reopening after dismiss keeps the same highlighted radio", async () => {
    const user = (await import("@testing-library/user-event")).default.setup();
    render(
      <MakeUpScreen
        plans={SHEET_PLANS}
        focusSessionKey={null}
        onUseTime={vi.fn()}
        onSeeMoreTimes={vi.fn()}
      />,
    );
    await user.click(screen.getByRole("button", { name: /choose a time/i }));
    const dialog = await screen.findByRole("dialog", { name: /choose a make-up time/i });
    const options = within(dialog).getAllByRole("radio");
    await user.click(options[1]);
    expect(options[1]).toHaveAttribute("aria-checked", "true");
    await user.click(screen.getByRole("button", { name: /close sheet/i }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: /choose a make-up time/i })).not.toBeInTheDocument();
    });
    const trigger = await screen.findByRole("button", { name: /choose a time/i });
    await waitFor(() => expect(trigger).toHaveFocus());
    await user.click(trigger);
    const reopened = await screen.findByRole("dialog", { name: /choose a make-up time/i });
    const reopenedOptions = within(reopened).getAllByRole("radio");
    expect(reopenedOptions[1]).toHaveAttribute("aria-checked", "true");
  });
});
