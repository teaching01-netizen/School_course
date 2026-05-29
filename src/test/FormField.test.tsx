import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import FormField from "@/components/ui/FormField";

describe("FormField", () => {
  it("renders hint text when hint prop is provided", () => {
    render(
      <FormField name="teacher_id" label="Teacher" hint="Teacher has a conflicting session">
        <input />
      </FormField>,
    );
    expect(screen.getByTestId("field-hint-teacher_id")).toHaveTextContent("Teacher has a conflicting session");
  });

  it("does not render hint when hint prop is not provided", () => {
    render(
      <FormField name="course_id" label="Course">
        <input />
      </FormField>,
    );
    expect(screen.queryByTestId("field-hint-course_id")).not.toBeInTheDocument();
  });
});
