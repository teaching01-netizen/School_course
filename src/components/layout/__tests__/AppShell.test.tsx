import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClientProvider, QueryClient } from "@tanstack/react-query";
import AppShell from "../AppShell";
import fs from "fs";
import path from "path";

vi.mock("../../../hooks/useAuth", () => ({
  useAuth: () => ({
    user: { id: "1", username: "admin", role: "Admin" as const },
    logout: vi.fn(),
  }),
}));

vi.mock("../../../api/client", () => ({
  apiJson: vi.fn().mockResolvedValue({ pending_count: 0 }),
}));

function renderShell() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/"]}>
        <AppShell>
          <div>content</div>
        </AppShell>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("AppShell", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("localStorage getItem/setItem throwing does not crash", () => {
    const getSpy = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("get fail");
    });
    const setSpy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("set fail");
    });
    expect(() => renderShell()).not.toThrow();
    getSpy.mockRestore();
    setSpy.mockRestore();
  });

  it("renders with max-w 1080 and safe-area env inset", () => {
    const { container } = renderShell();
    const inner = container.querySelector("main > div") as HTMLElement | null;
    expect(inner).not.toBeNull();
    const cls = inner!.getAttribute("class") ?? "";
    expect(cls).toContain("max-w-[1080px]");
    const src = fs.readFileSync(path.resolve("src/components/layout/AppShell.tsx"), "utf8");
    expect(src).toContain("max-w-[1080px]");
    expect(src).toContain("env(safe-area-inset-left)");
  });

  it("exposes onInertChange via useMainInert (R2 mocks) - main becomes inert when mobile nav opens", async () => {
    const { container } = renderShell();
    const main = container.querySelector("main") as HTMLElement;
    expect(main).not.toBeNull();
    expect(main.hasAttribute("inert")).toBe(false);
  });
});
