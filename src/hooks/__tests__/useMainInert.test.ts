import { describe, it, expect, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useMainInert } from "../useMainInert";

function mountMain() {
  document.body.style.overflow = "";
  const main = document.createElement("main");
  document.body.appendChild(main);
  return main;
}
function cleanupMain() {
  document.body.style.overflow = "";
  for (const el of [...document.querySelectorAll("main")]) el.remove();
}

describe("useMainInert", () => {
  beforeEach(() => {
    cleanupMain();
  });

  it("dual refcount drawer(1/1) -> panel(2/2) -> close drawer 1/1 -> close panel 0/0 restores overflow and inert", () => {
    const main = mountMain();
    document.body.style.overflow = "auto";
    const { result } = renderHook(() => useMainInert());
    result.current.add("drawer");
    expect(main.hasAttribute("inert") || main.getAttribute("aria-hidden") === "true").toBe(true);
    expect(document.body.style.overflow).toBe("hidden");
    result.current.add("panel");
    expect(main.hasAttribute("inert") || main.getAttribute("aria-hidden") === "true").toBe(true);
    expect(document.body.style.overflow).toBe("hidden");
    result.current.remove("drawer");
    expect(main.hasAttribute("inert") || main.getAttribute("aria-hidden") === "true").toBe(true);
    expect(document.body.style.overflow).toBe("hidden");
    result.current.remove("panel");
    expect(main.hasAttribute("inert")).toBe(false);
    expect(main.hasAttribute("aria-hidden")).toBe(false);
    expect(document.body.style.overflow).toBe("auto");
  });

  it("underflow remove('ghost') no throw on both maps", () => {
    mountMain();
    const { result } = renderHook(() => useMainInert());
    expect(() => result.current.remove("ghost")).not.toThrow();
    expect(() => result.current.remove("ghost")).not.toThrow();
  });

  it("deduped Set cleanup removes each key once across both maps on unmount", () => {
    const main = mountMain();
    document.body.style.overflow = "auto";
    const { result, unmount } = renderHook(() => useMainInert());
    result.current.add("drawer");
    result.current.add("panel");
    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("auto");
    expect(main.hasAttribute("inert")).toBe(false);
  });

  it("SSR typeof document undefined no-op when document is absent", () => {
    const { result, unmount } = renderHook(() => useMainInert());
    const realDoc = globalThis.document;
    // Exercise the SSR branch by temporarily removing document for add/remove only
    const saved = (globalThis as unknown as { document?: unknown }).document;
    try {
      delete (globalThis as unknown as { document?: unknown }).document;
      // Verify typeof document is now undefined (proves guard would fire if hook were in real SSR)
      expect(typeof (globalThis as unknown as { document?: unknown }).document).toBe("undefined");
      // Call add/remove - these should early-return via the guard and not throw
      expect(() => result.current.add("x")).not.toThrow();
      expect(() => result.current.remove("x")).not.toThrow();
    } finally {
      (globalThis as unknown as { document?: unknown }).document = saved ?? realDoc;
    }
    // Verify that after restoring, hook still functions normally
    expect(() => result.current.add("x2")).not.toThrow();
    expect(() => result.current.remove("x2")).not.toThrow();
    unmount();
  });

  it("single previousOverflow restore — second opener does not overwrite stored value", () => {
    const main = mountMain();
    document.body.style.overflow = "scroll";
    const { result } = renderHook(() => useMainInert());
    result.current.add("a");
    expect(document.body.style.overflow).toBe("hidden");
    result.current.add("b");
    result.current.remove("a");
    expect(document.body.style.overflow).toBe("hidden");
    result.current.remove("b");
    expect(document.body.style.overflow).toBe("scroll");
    expect(main.hasAttribute("inert")).toBe(false);
  });

  it("fallback aria-hidden when inert is not in prototype", () => {
    const proto = HTMLElement.prototype as unknown as { inert?: unknown };
    const had = "inert" in HTMLElement.prototype;
    const saved = proto.inert;
    try {
      // eslint-disable-next-line @typescript-eslint/no-dynamic-delete
      if (had) delete proto.inert;
      cleanupMain();
      const main = mountMain();
      const { result } = renderHook(() => useMainInert());
      result.current.add("x");
      expect(main.getAttribute("aria-hidden")).toBe("true");
      expect(main.hasAttribute("inert")).toBe(false);
      result.current.remove("x");
      expect(main.hasAttribute("aria-hidden")).toBe(false);
    } finally {
      if (had) Object.defineProperty(HTMLElement.prototype, "inert", { value: saved, configurable: true, writable: true });
    }
  });

  it("clamped Math.max keeps counts non-negative across maps", () => {
    const { result } = renderHook(() => useMainInert());
    result.current.add("k");
    result.current.remove("k");
    result.current.remove("k");
    result.current.remove("k");
    expect(() => result.current.add("k")).not.toThrow();
  });
});
