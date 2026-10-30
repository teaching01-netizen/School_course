import { describe, expect, it } from "vitest";
import type { SitInDisplayModel } from "@/features/absences/domain/sitInResolution";
import type { MakeUpOption } from "../MakeUpPicker";
import { defaultMakeUpDay, firstAddedOptionDay, makeUpDays, optionsForDay } from "../makeUpDays";

function display(dateKey: string, overrides: Partial<SitInDisplayModel> = {}): SitInDisplayModel {
  return {
    className: "Calculus III",
    subjectName: "Mathematics",
    teacherName: "AJ. Nice",
    date: "display only",
    dateKey,
    startTime: "09:00",
    endTime: "10:00",
    status: "available",
    ...overrides,
  };
}

function option(value: string, dateKey?: string, overrides: Partial<MakeUpOption> = {}): MakeUpOption {
  return { value, label: value, display: dateKey ? display(dateKey) : undefined, ...overrides };
}

describe("makeUpDays", () => {
  it("groups options into chronological institute days and marks days with no selectable time", () => {
    const days = makeUpDays([
      option("mon", "2026-01-19"),
      option("sat", "2026-01-17"),
      option("sun", "2026-01-18", { disabled: true, description: "Overlaps another scheduled class" }),
      option("sun-late", "2026-01-18", { disabled: true }),
    ]);

    expect(days.map((day) => day.key)).toEqual(["2026-01-17", "2026-01-18", "2026-01-19"]);
    expect(days.map((day) => day.weekday)).toEqual(["Sat", "Sun", "Mon"]);
    expect(days.map((day) => day.dayNumber)).toEqual(["17", "18", "19"]);
    expect(days.every((day) => day.monthLabel === "January 2026")).toBe(true);
    expect(days.map((day) => day.ariaLabel)).toEqual(["Saturday 17 January", "Sunday 18 January", "Monday 19 January"]);
    expect(days[0]).toMatchObject({ total: 1, availableCount: 1, hasAvailable: true });
    expect(days[1]).toMatchObject({ total: 2, availableCount: 0, hasAvailable: false });
  });

  it("reports a different month for a day list that crosses a month boundary", () => {
    const days = makeUpDays([option("jan", "2026-01-31"), option("feb", "2026-02-01")]);
    expect(days.map((day) => day.monthShort)).toEqual(["Jan", "Feb"]);
    expect(days.map((day) => day.monthLabel)).toEqual(["January 2026", "February 2026"]);
    expect(days[0].weekday).toBe("Sat");
    expect(days[1].weekday).toBe("Sun");
  });

  it("returns no day model when no option carries a day key", () => {
    expect(makeUpDays([option("a"), option("b")])).toEqual([]);
  });

  it("ignores key-less options when other options do carry day keys", () => {
    const days = makeUpDays([option("legacy"), option("dated", "2026-01-17")]);
    expect(days.map((day) => day.key)).toEqual(["2026-01-17"]);
    expect(days[0].total).toBe(1);
  });
});

describe("defaultMakeUpDay", () => {
  const options = [option("sat", "2026-01-17"), option("sun", "2026-01-18", { disabled: true }), option("mon", "2026-01-19")];
  const days = makeUpDays(options);

  it("opens on the day of the committed choice even when it became unavailable", () => {
    expect(defaultMakeUpDay(options, days, "sun")).toBe("2026-01-18");
  });

  it("opens on the first day with a selectable time otherwise", () => {
    expect(defaultMakeUpDay(options, days, "")).toBe("2026-01-17");
    expect(defaultMakeUpDay(options, days, "unknown-value")).toBe("2026-01-17");
  });

  it("skips a fully unavailable first day when nothing is staged", () => {
    const unavailableFirst = [option("sat", "2026-01-17", { disabled: true }), option("mon", "2026-01-19")];
    expect(defaultMakeUpDay(unavailableFirst, makeUpDays(unavailableFirst), "")).toBe("2026-01-19");
  });

  it("returns an empty day when there is no day model", () => {
    expect(defaultMakeUpDay([option("a")], [], "")).toBe("");
  });
});

describe("optionsForDay", () => {
  const options = [option("sat-disabled", "2026-01-17", { disabled: true }), option("sat", "2026-01-17"), option("mon", "2026-01-19"), option("legacy")];

  it("keeps the staff order and returns every option for the flat list", () => {
    expect(optionsForDay(options, "2026-01-17", false).map((item) => item.value)).toEqual(["sat-disabled", "sat", "legacy"]);
    expect(optionsForDay(options, "", false).map((item) => item.value)).toEqual(["sat-disabled", "sat", "mon", "legacy"]);
  });

  it("sorts available times first for the public form", () => {
    expect(optionsForDay(options, "2026-01-17", true).map((item) => item.value)).toEqual(["sat", "legacy", "sat-disabled"]);
    expect(optionsForDay(options, "", true).map((item) => item.value)).toEqual(["sat", "mon", "legacy", "sat-disabled"]);
  });
});

describe("firstAddedOptionDay", () => {
  const options = [option("sat", "2026-01-17"), option("sun", "2026-01-18")];

  it("reports the day of the first option added by a reveal", () => {
    expect(firstAddedOptionDay(options, new Set(["sat"]))).toBe("2026-01-18");
  });

  it("reports nothing when the reveal added no option or the new option has no day", () => {
    expect(firstAddedOptionDay(options, new Set(["sat", "sun"]))).toBeNull();
    expect(firstAddedOptionDay([...options, option("legacy")], new Set(["sat", "sun"]))).toBeNull();
  });
});
