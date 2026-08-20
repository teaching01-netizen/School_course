import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useVisualViewport } from "./useVisualViewport";

type FakeVisualViewport = EventTarget & {
  height: number;
  offsetTop: number;
  width: number;
};

function ViewportProbe() {
  const viewport = useVisualViewport();
  return (
    <output data-testid="viewport">
      {JSON.stringify(viewport)}
    </output>
  );
}

describe("useVisualViewport", () => {
  let fakeViewport: FakeVisualViewport;

  beforeEach(() => {
    fakeViewport = Object.assign(new EventTarget(), {
      height: 844,
      offsetTop: 0,
      width: 390,
    });
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: fakeViewport,
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 844,
    });
  });

  afterEach(() => {
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: undefined,
    });
  });

  it("uses the visual viewport and identifies a keyboard-sized resize", () => {
    render(<ViewportProbe />);
    expect(screen.getByTestId("viewport")).toHaveTextContent(
      JSON.stringify({ height: 844, offsetTop: 0, keyboardLikelyOpen: false }),
    );

    act(() => {
      fakeViewport.height = 420;
      fakeViewport.offsetTop = 12;
      fakeViewport.dispatchEvent(new Event("resize"));
    });

    expect(screen.getByTestId("viewport")).toHaveTextContent(
      JSON.stringify({ height: 420, offsetTop: 12, keyboardLikelyOpen: true }),
    );
  });

  it("falls back to the layout viewport when visualViewport is unavailable", () => {
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: undefined,
    });
    render(<ViewportProbe />);

    expect(screen.getByTestId("viewport")).toHaveTextContent(
      JSON.stringify({ height: 844, offsetTop: 0, keyboardLikelyOpen: false }),
    );
  });
});
