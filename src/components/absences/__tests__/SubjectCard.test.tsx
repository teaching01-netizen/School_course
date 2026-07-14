import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import SubjectCard from "../SubjectCard";

describe("SubjectCard", () => {
  it("shows the subject name", () => {
    render(<SubjectCard id="subj-1" name="Mathematics" selected={false} onToggle={() => {}} />);
    expect(screen.getByText("Mathematics")).toBeInTheDocument();
  });

  it("does not display the subject code", () => {
    render(<SubjectCard id="subj-1" name="Mathematics" selected={false} onToggle={() => {}} />);
    expect(screen.queryByText("MATH")).not.toBeInTheDocument();
  });
});
