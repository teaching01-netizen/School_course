import { act, render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { reconnectDelay, RealtimeProvider } from "./RealtimeProvider";
import { useRealtime } from "@/hooks/useRealtime";

type Listener = (event: { data?: string }) => void;

class FakeWebSocket {
  static OPEN = 1;
  static instances: FakeWebSocket[] = [];

  readyState = 0;
  sent: string[] = [];
  private listeners = new Map<string, Set<Listener>>();

  constructor(public readonly url: string) {
    FakeWebSocket.instances.push(this);
  }

  addEventListener(type: string, listener: Listener) {
    const listeners = this.listeners.get(type) ?? new Set<Listener>();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  send(value: string) {
    this.sent.push(value);
  }

  close() {
    this.readyState = 3;
  }

  emit(type: string, data?: string) {
    if (type === "open") this.readyState = FakeWebSocket.OPEN;
    for (const listener of this.listeners.get(type) ?? []) listener({ data });
  }
}

function ListenerComponent({ channel, onEvent }: { channel: string; onEvent: (event: unknown) => void }) {
  useRealtime([channel], onEvent);
  return null;
}

describe("RealtimeProvider", () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal("WebSocket", FakeWebSocket);
  });

  it("uses capped jitter for reconnect delays", () => {
    expect(reconnectDelay(1, () => 0)).toBe(500);
    expect(reconnectDelay(1, () => 1)).toBe(1_000);
    expect(reconnectDelay(20, () => 1)).toBe(30_000);
  });

  it("shares one socket and reference-counts channel subscriptions", async () => {
    const onFirst = vi.fn();
    const onSecond = vi.fn();
    const { rerender } = render(
      <RealtimeProvider enabled>
        <ListenerComponent channel="sessions:all" onEvent={onFirst} />
        <ListenerComponent channel="sessions:all" onEvent={onSecond} />
      </RealtimeProvider>,
    );

    expect(FakeWebSocket.instances).toHaveLength(1);
    const socket = FakeWebSocket.instances[0];
    act(() => socket.emit("open"));
    expect(socket.sent).toEqual([JSON.stringify({ type: "subscribe", channel: "sessions:all" })]);

    act(() => socket.emit("message", JSON.stringify({ type: "session.updated", channel: "sessions:all", id: "s-1" })));
    expect(onFirst).toHaveBeenCalledTimes(1);
    expect(onSecond).toHaveBeenCalledTimes(1);

    rerender(
      <RealtimeProvider enabled>
        <ListenerComponent channel="sessions:all" onEvent={onFirst} />
      </RealtimeProvider>,
    );
    expect(socket.sent).toHaveLength(1);

    rerender(<RealtimeProvider enabled>{null}</RealtimeProvider>);
    await waitFor(() => {
      expect(socket.sent).toContain(JSON.stringify({ type: "unsubscribe", channel: "sessions:all" }));
    });
  });

  it("reconnects, resubscribes, and reports a recovered connection once", async () => {
    vi.useFakeTimers();
    const onReconnect = vi.fn();
    render(
      <RealtimeProvider enabled onReconnect={onReconnect}>
        <ListenerComponent channel="absent:all" onEvent={vi.fn()} />
      </RealtimeProvider>,
    );

    const first = FakeWebSocket.instances[0];
    act(() => first.emit("open"));
    expect(onReconnect).not.toHaveBeenCalled();
    act(() => first.emit("close"));
    await act(async () => vi.advanceTimersByTimeAsync(1_000));

    expect(FakeWebSocket.instances).toHaveLength(2);
    const second = FakeWebSocket.instances[1];
    act(() => second.emit("open"));
    expect(second.sent).toEqual([JSON.stringify({ type: "subscribe", channel: "absent:all" })]);
    expect(onReconnect).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  it("reports recovery when the initial connection fails before opening", async () => {
    vi.useFakeTimers();
    const onReconnect = vi.fn();
    render(<RealtimeProvider enabled onReconnect={onReconnect}>{null}</RealtimeProvider>);

    act(() => FakeWebSocket.instances[0].emit("close"));
    await act(async () => vi.advanceTimersByTimeAsync(1_000));
    act(() => FakeWebSocket.instances[1].emit("open"));

    expect(onReconnect).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });
});
