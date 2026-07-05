import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDate, formatDateTime, formatTime } from "../date";

describe("institute date formatting", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("formats session times in Bangkok instead of the browser timezone", () => {
    const spy = vi.spyOn(Date.prototype, "toLocaleTimeString");

    expect(formatTime("2026-01-15T02:00:00Z")).toBe("09:00");
    expect(spy).toHaveBeenCalledWith(
      "en-GB",
      expect.objectContaining({ timeZone: "Asia/Bangkok" }),
    );
  });

  it("formats timestamp dates on the Bangkok calendar day", () => {
    const spy = vi.spyOn(Date.prototype, "toLocaleDateString");
    const label = formatDate("2026-01-15T17:00:00Z");

    expect(label).toContain("16 Jan 2026");
    expect(spy).toHaveBeenCalledWith(
      "en-GB",
      expect.objectContaining({ timeZone: "Asia/Bangkok" }),
    );
  });

  it("formats session datetimes on the Bangkok calendar day", () => {
    const spy = vi.spyOn(Date.prototype, "toLocaleString");
    const label = formatDateTime("2026-01-15T17:00:00Z");

    expect(label).toContain("16 Jan");
    expect(label).toContain("00:00");
    expect(spy).toHaveBeenCalledWith(
      "en-GB",
      expect.objectContaining({ timeZone: "Asia/Bangkok" }),
    );
  });
});
