import type { ReactElement } from "react";
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ErrorBoundary, isChunkLoadError, reloadForNewBuild } from "../ErrorBoundary";

// jsdom does not expose sessionStorage for opaque origins (same as localStorage
// in src/test/setup.ts); the cooldown guard reads and writes it.
function stubSessionStorage(store: Map<string, string> | null): void {
  Object.defineProperty(window, "sessionStorage", {
    configurable: true,
    get: () => {
      if (!store) throw new DOMException("Storage unavailable", "SecurityError");
      return {
        getItem: (key: string) => store.get(key) ?? null,
        setItem: (key: string, value: string) => void store.set(key, String(value)),
        removeItem: (key: string) => void store.delete(key),
      };
    },
  });
}

function Bomb({ error }: { error: Error }): ReactElement {
  throw error;
}

beforeEach(() => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  stubSessionStorage(new Map());
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("isChunkLoadError", () => {
  it.each([
    "Failed to fetch dynamically imported module: http://localhost/assets/Courses-B1x.js",
    "error loading dynamically imported module",
    "Importing a module script failed.",
    "Loading chunk 42 failed.",
    "Loading CSS chunk 7 failed.",
  ])("recognizes %s", (message) => {
    expect(isChunkLoadError(new Error(message))).toBe(true);
  });

  it("rejects unrelated errors and non-error values", () => {
    expect(isChunkLoadError(new TypeError("Cannot read properties of undefined"))).toBe(false);
    expect(isChunkLoadError(undefined)).toBe(false);
    expect(isChunkLoadError(null)).toBe(false);
  });
});

describe("ErrorBoundary", () => {
  it("renders children while healthy", () => {
    render(
      <ErrorBoundary>
        <p>App content</p>
      </ErrorBoundary>,
    );
    expect(screen.getByText("App content")).toBeInTheDocument();
  });

  it("recovers a chunk-load error by reloading the new build", () => {
    const recover = vi.fn(() => true);
    render(
      <ErrorBoundary recoverFromChunkError={recover}>
        <Bomb error={new Error("Failed to fetch dynamically imported module: /assets/Home-1.js")} />
      </ErrorBoundary>,
    );
    expect(recover).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("status")).toHaveTextContent("relaunching");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("falls back to the manual screen when auto-recovery is unavailable", () => {
    const recover = vi.fn(() => false);
    render(
      <ErrorBoundary recoverFromChunkError={recover}>
        <Bomb error={new Error("Failed to fetch dynamically imported module: /assets/Home-1.js")} />
      </ErrorBoundary>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong");
    expect(screen.getByRole("button", { name: "Reload page" })).toBeInTheDocument();
  });

  it("shows the manual screen for non-chunk errors without auto-recovery", () => {
    const recover = vi.fn(() => true);
    render(
      <ErrorBoundary recoverFromChunkError={recover}>
        <Bomb error={new TypeError("Cannot read properties of undefined (reading 'id')")} />
      </ErrorBoundary>,
    );
    expect(recover).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("Something went wrong");
  });
});

describe("reloadForNewBuild", () => {
  it("triggers recovery when no recent reload is recorded", () => {
    expect(reloadForNewBuild()).toBe(true);
  });

  it("refuses to reload again within the cooldown window", () => {
    expect(reloadForNewBuild()).toBe(true);
    expect(reloadForNewBuild()).toBe(false);
  });

  it("resumes after the cooldown passes", () => {
    const store = new Map<string, string>([["wi.chunk-reload-at", String(Date.now() - 31_000)]]);
    stubSessionStorage(store);
    expect(reloadForNewBuild()).toBe(true);
  });

  it("never auto-reloads when storage is unavailable", () => {
    stubSessionStorage(null);
    expect(reloadForNewBuild()).toBe(false);
  });
});
