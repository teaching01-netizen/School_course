import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import AbsenceAppShell from "../AbsenceAppShell";

describe("AbsenceAppShell", () => {
  it("keeps header, scrollable main content, and actions in app landmarks", () => {
    render(
      <AbsenceAppShell
        header={<div>Header</div>}
        footer={<button type="button">Continue</button>}
      >
        <h1>Report an absence</h1>
      </AbsenceAppShell>,
    );

    expect(screen.getByRole("banner")).toHaveTextContent("Header");
    expect(screen.getByRole("main")).toHaveTextContent("Report an absence");
    expect(screen.getByRole("contentinfo")).toContainElement(screen.getByRole("button", { name: "Continue" }));
    expect(screen.getByRole("main")).toHaveAttribute("tabindex", "-1");
  });
});
