import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import Sidebar from "../Sidebar";

vi.mock("../../../hooks/useAuth", () => ({
  useAuth: () => ({
    user: { id: "1", username: "admin", role: "Admin" as const },
    logout: vi.fn().mockResolvedValue(undefined),
  }),
}));

vi.mock("../../../api/client", () => ({
  apiJson: vi.fn().mockResolvedValue({ pending_count: 0 }),
}));

function renderSidebar(props: Partial<React.ComponentProps<typeof Sidebar>> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/"]}>
        <Sidebar collapsed={false} mobileOpen={false} onCloseMobile={vi.fn()} {...props} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Sidebar", () => {
  const originalInnerWidth = window.innerWidth;
  beforeEach(() => {
    try { localStorage.clear(); } catch {}
    document.body.style.overflow = "";
    try { Object.defineProperty(window, "innerWidth", { value: originalInnerWidth, configurable: true }); } catch {}
  });

  it("resize handle has tabIndex 0 and aria-valuenow reflects width", () => {
    renderSidebar();
    const handle = screen.getByRole("separator", { name: /resize sidebar/i });
    expect(handle.getAttribute("tabindex")).toBe("0");
    expect(handle.getAttribute("aria-valuenow")).toBe("252");
    expect(handle.getAttribute("aria-valuemin")).toBe("220");
    expect(handle.getAttribute("aria-valuemax")).toBe("420");
    expect(handle.getAttribute("aria-valuetext")).toBe("252px");
    expect(handle.getAttribute("aria-orientation")).toBe("vertical");
  });

  it("resize handle has focus-visible:ring class", () => {
    renderSidebar();
    const handle = screen.getByRole("separator", { name: /resize sidebar/i });
    expect(handle.className).toContain("focus-visible:ring");
  });

  it("ArrowLeft -10 clamped to 220, ArrowRight +10, Home->220 End->420, dblclick->252", () => {
    renderSidebar();
    const handle = screen.getByRole("separator", { name: /resize sidebar/i });
    fireEvent.keyDown(handle, { key: "ArrowLeft" });
    expect(handle.getAttribute("aria-valuenow")).toBe("242");
    // clamp to 220: press many times
    for (let i = 0; i < 10; i++) fireEvent.keyDown(handle, { key: "ArrowLeft" });
    expect(handle.getAttribute("aria-valuenow")).toBe("220");
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(handle.getAttribute("aria-valuenow")).toBe("230");
    fireEvent.keyDown(handle, { key: "Home" });
    expect(handle.getAttribute("aria-valuenow")).toBe("220");
    fireEvent.keyDown(handle, { key: "End" });
    expect(handle.getAttribute("aria-valuenow")).toBe("420");
    fireEvent.doubleClick(handle);
    expect(handle.getAttribute("aria-valuenow")).toBe("252");
  });

  it("window resize clamps width via Math.min(width, innerWidth-80) and [220,420]", () => {
    renderSidebar();
    const handle = screen.getByRole("separator", { name: /resize sidebar/i });
    // set width to 420 via End
    fireEvent.keyDown(handle, { key: "End" });
    expect(handle.getAttribute("aria-valuenow")).toBe("420");
    // innerWidth 300 -> max available 220 (300-80), should clamp down
    Object.defineProperty(window, "innerWidth", { value: 300, configurable: true });
    act(() => { window.dispatchEvent(new Event("resize")); });
    expect(handle.getAttribute("aria-valuenow")).toBe("220");
    // innerWidth 800 -> max 720 but capped at MAX_WIDTH 420, stays 220
    Object.defineProperty(window, "innerWidth", { value: 800, configurable: true });
    act(() => { window.dispatchEvent(new Event("resize")); });
    expect(handle.getAttribute("aria-valuenow")).toBe("220");
    // bump to 252 then resize with large window keeps it
    fireEvent.doubleClick(handle);
    expect(handle.getAttribute("aria-valuenow")).toBe("252");
    act(() => { window.dispatchEvent(new Event("resize")); });
    expect(handle.getAttribute("aria-valuenow")).toBe("252");
  });

  it("localStorage getItem/setItem throwing does not crash (try/catch per file)", () => {
    const getSpy = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("get fail");
    });
    const setSpy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("set fail");
    });
    expect(() => renderSidebar()).not.toThrow();
    const handle = screen.getByRole("separator", { name: /resize sidebar/i });
    expect(() => fireEvent.keyDown(handle, { key: "ArrowRight" })).not.toThrow();
    getSpy.mockRestore();
    setSpy.mockRestore();
  });
});
