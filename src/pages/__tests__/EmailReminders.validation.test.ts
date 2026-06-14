import { describe, expect, it } from "vitest";
import { validateEmailTemplateForm } from "../EmailReminders";

describe("EmailReminders template validation", () => {
  it("rejects blank subject lines", () => {
    expect(validateEmailTemplateForm({
      name: "Sit-in",
      subject: "   ",
      body: "Body",
      requireName: true,
    })).toBe("Subject line is required");
  });

  it("rejects blank preview bodies without requiring a template name", () => {
    expect(validateEmailTemplateForm({
      subject: "Subject",
      body: "\t",
      requireName: false,
    })).toBe("Email body is required");
  });

  it("accepts valid fields with surrounding whitespace", () => {
    expect(validateEmailTemplateForm({
      name: " Sit-in ",
      subject: " Subject ",
      body: " Body ",
      requireName: true,
    })).toBeNull();
  });
});
