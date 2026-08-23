import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useConnectivity } from "./useConnectivity";

function ConnectivityProbe() {
  const state = useConnectivity();
  return <output data-testid="connectivity">{JSON.stringify(state)}</output>;
}

describe("useConnectivity", () => {
  beforeEach(() => {
    Object.defineProperty(navigator, "onLine", { configurable: true, value: true });
  });

  afterEach(() => {
    Object.defineProperty(navigator, "onLine", { configurable: true, value: true });
  });

  it("moves offline and restores a transient reconnect state", async () => {
    render(<ConnectivityProbe />);
    expect(screen.getByTestId("connectivity")).toHaveTextContent('{"online":true,"justRestored":false}');

    act(() => window.dispatchEvent(new Event("offline")));
    expect(screen.getByTestId("connectivity")).toHaveTextContent('{"online":false,"justRestored":false}');

    act(() => window.dispatchEvent(new Event("online")));
    expect(screen.getByTestId("connectivity")).toHaveTextContent('{"online":true,"justRestored":true}');
  });

  it("does not trust a stale false navigator hint on startup", () => {
    Object.defineProperty(navigator, "onLine", { configurable: true, value: false });

    render(<ConnectivityProbe />);

    expect(screen.getByTestId("connectivity")).toHaveTextContent('{"online":true,"justRestored":false}');
  });
});
