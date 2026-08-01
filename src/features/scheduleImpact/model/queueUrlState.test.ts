import { describe, expect, it } from "vitest";
import { parseQueueParams, serializeQueueParams, resetOffsetOnFilterChange } from "./queueUrlState";
import type { QueueURLState } from "../types";

describe("parseQueueParams", () => {
  it("missing status defaults to 'all'", () => {
    expect(parseQueueParams(new URLSearchParams({})).status).toBe("all");
  });

  it("invalid offset defaults to zero", () => {
    expect(parseQueueParams(new URLSearchParams({ offset: "abc" })).offset).toBe(0);
  });

  it("NaN offset defaults to zero", () => {
    expect(parseQueueParams(new URLSearchParams({ offset: "not-a-number" })).offset).toBe(0);
  });

  it("invalid limit defaults to 25", () => {
    expect(parseQueueParams(new URLSearchParams({ limit: "999" })).limit).toBe(25);
  });

  it("limit 50 is accepted", () => {
    expect(parseQueueParams(new URLSearchParams({ limit: "50" })).limit).toBe(50);
  });

  it("limit 100 is accepted", () => {
    expect(parseQueueParams(new URLSearchParams({ limit: "100" })).limit).toBe(100);
  });

  it("search input is preserved as-is in query field", () => {
    expect(parseQueueParams(new URLSearchParams({ q: "  Alice  " })).query).toBe("  Alice  ");
  });

  it("view defaults to 'queue' for unknown values", () => {
    expect(parseQueueParams(new URLSearchParams({ view: "unknown" })).view).toBe("queue");
  });

  it("view 'processing' is accepted", () => {
    expect(parseQueueParams(new URLSearchParams({ view: "processing" })).view).toBe("processing");
  });

  it("view 'history' is accepted", () => {
    expect(parseQueueParams(new URLSearchParams({ view: "history" })).view).toBe("history");
  });

  it("all fields parsed correctly together", () => {
    const params = new URLSearchParams({
      view: "processing",
      q: "test",
      severity: "critical",
      status: "open",
      offset: "25",
      limit: "50",
    });
    const state = parseQueueParams(params);
    expect(state).toEqual({
      view: "processing",
      query: "test",
      severity: "critical",
      status: "open",
      offset: 25,
      limit: 50,
    });
  });
});

describe("serializeQueueParams", () => {
  it("default state produces minimal params", () => {
    const state: QueueURLState = {
      view: "queue", query: "", severity: "", status: "all", offset: 0, limit: 25,
    };
    expect(serializeQueueParams(state).toString()).toBe("");
  });

  it("view 'processing' is serialized", () => {
    const state: QueueURLState = {
      view: "processing", query: "", severity: "", status: "all", offset: 0, limit: 25,
    };
    expect(serializeQueueParams(state).get("view")).toBe("processing");
  });

  it("non-zero offset is serialized", () => {
    const state: QueueURLState = {
      view: "queue", query: "", severity: "", status: "all", offset: 25, limit: 25,
    };
    expect(serializeQueueParams(state).get("offset")).toBe("25");
  });

  it("limit 50 is serialized", () => {
    const state: QueueURLState = {
      view: "queue", query: "", severity: "", status: "all", offset: 0, limit: 50,
    };
    expect(serializeQueueParams(state).get("limit")).toBe("50");
  });

  it("query is serialized", () => {
    const state: QueueURLState = {
      view: "queue", query: "Alice", severity: "", status: "all", offset: 0, limit: 25,
    };
    expect(serializeQueueParams(state).get("q")).toBe("Alice");
  });

  it("severity is serialized", () => {
    const state: QueueURLState = {
      view: "queue", query: "", severity: "critical", status: "all", offset: 0, limit: 25,
    };
    expect(serializeQueueParams(state).get("severity")).toBe("critical");
  });

  it("status 'open' is serialized", () => {
    const state: QueueURLState = {
      view: "queue", query: "", severity: "", status: "open", offset: 0, limit: 25,
    };
    expect(serializeQueueParams(state).get("status")).toBe("open");
  });
});

describe("resetOffsetOnFilterChange", () => {
  it("always returns 0", () => {
    expect(resetOffsetOnFilterChange()).toBe(0);
  });
});

describe("round-trip parsing and serialization", () => {
  it("serialize then parse preserves state", () => {
    const original: QueueURLState = {
      view: "processing", query: "test", severity: "critical", status: "open", offset: 25, limit: 50,
    };
    const params = serializeQueueParams(original);
    const parsed = parseQueueParams(params);
    expect(parsed).toEqual(original);
  });
});
