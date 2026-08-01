import { describe, expect, it } from "vitest";
import { computePageRange, normalizeOffset, nextOffset, prevOffset } from "./pagination";

describe("computePageRange", () => {
  it("first page: 1–25 of 73", () => {
    expect(computePageRange(0, 25, 73)).toEqual({ start: 1, end: 25, total: 73 });
  });

  it("middle page: 26–50 of 73", () => {
    expect(computePageRange(25, 25, 73)).toEqual({ start: 26, end: 50, total: 73 });
  });

  it("last page: 51–73 of 73", () => {
    expect(computePageRange(50, 25, 73)).toEqual({ start: 51, end: 73, total: 73 });
  });

  it("empty result: 0–0 of 0", () => {
    expect(computePageRange(0, 25, 0)).toEqual({ start: 0, end: 0, total: 0 });
  });

  it("single item page shows 1–1 of 1", () => {
    expect(computePageRange(0, 25, 1)).toEqual({ start: 1, end: 1, total: 1 });
  });

  it("exact page boundary shows correct range", () => {
    expect(computePageRange(0, 25, 25)).toEqual({ start: 1, end: 25, total: 25 });
  });

  it("page size 100 works correctly", () => {
    expect(computePageRange(0, 100, 150)).toEqual({ start: 1, end: 100, total: 150 });
  });

  it("offset beyond total clamps to last valid position", () => {
    // offset 100 on total 50 → clamps to offset 49 → start 50, end min(74,50)=50
    expect(computePageRange(100, 25, 50)).toEqual({ start: 50, end: 50, total: 50 });
  });

  it("negative offset clamps to 0", () => {
    expect(computePageRange(-5, 25, 50)).toEqual({ start: 1, end: 25, total: 50 });
  });
});

describe("normalizeOffset", () => {
  it("offset beyond total is normalized to last valid position", () => {
    expect(normalizeOffset(100, 50)).toBe(49);
  });

  it("negative offset becomes zero", () => {
    expect(normalizeOffset(-5, 50)).toBe(0);
  });

  it("valid offset passes through", () => {
    expect(normalizeOffset(25, 73)).toBe(25);
  });

  it("zero offset passes through", () => {
    expect(normalizeOffset(0, 50)).toBe(0);
  });
});

describe("nextOffset", () => {
  it("adds limit to current offset", () => {
    expect(nextOffset(0, 25)).toBe(25);
    expect(nextOffset(25, 50)).toBe(75);
  });
});

describe("prevOffset", () => {
  it("subtracts limit and never goes negative", () => {
    expect(prevOffset(25, 25)).toBe(0);
    expect(prevOffset(50, 25)).toBe(25);
    expect(prevOffset(0, 25)).toBe(0);
  });
});
