import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RealtimeContext } from "@/realtime/RealtimeProvider";
import { useRealtime, type RealtimeEvent } from "./useRealtime";

type Handler = (event: RealtimeEvent) => void;

function createHarness() {
  const handlers = new Map<string, Set<Handler>>();
  const reconnectHandlers = new Set<() => void>();
  const subscribe = (channel: string, handler: Handler) => {
    const channelHandlers = handlers.get(channel) ?? new Set<Handler>();
    channelHandlers.add(handler);
    handlers.set(channel, channelHandlers);
    return () => {
      channelHandlers.delete(handler);
      if (channelHandlers.size === 0) handlers.delete(channel);
    };
  };
  const contextValue = {
    subscribe,
    subscribeReconnect: (handler: () => void) => {
      reconnectHandlers.add(handler);
      return () => reconnectHandlers.delete(handler);
    },
  };
  const wrapper = ({ children }: { children: ReactNode }) => (
    <RealtimeContext.Provider value={contextValue}>
      {children}
    </RealtimeContext.Provider>
  );
  const emit = (event: RealtimeEvent) => {
    for (const handler of handlers.get(event.channel) ?? []) handler(event);
  };
  const reconnect = () => {
    for (const handler of reconnectHandlers) handler();
  };
  return { wrapper, emit, reconnect };
}

afterEach(() => {
  vi.useRealTimers();
});

describe("useRealtime event batching", () => {
  it("delivers distinct resource IDs from the same debounce window", async () => {
    vi.useFakeTimers();
    const harness = createHarness();
    const onEvent = vi.fn();
    renderHook(() => useRealtime(["absent:all"], onEvent, { debounceMs: 100 }), { wrapper: harness.wrapper });

    act(() => {
      harness.emit({ type: "absence.updated", channel: "absent:all", id: "absence-1" });
      harness.emit({ type: "absence.updated", channel: "absent:all", id: "absence-2" });
    });
    await act(async () => vi.advanceTimersByTimeAsync(100));

    expect(onEvent.mock.calls.map(([event]) => event.id)).toEqual(["absence-1", "absence-2"]);
  });

  it("delivers distinct channels from the same debounce window", async () => {
    vi.useFakeTimers();
    const harness = createHarness();
    const onEvent = vi.fn();
    renderHook(() => useRealtime(["sessions:all", "courses:all"], onEvent, { debounceMs: 100 }), {
      wrapper: harness.wrapper,
    });

    act(() => {
      harness.emit({ type: "session.updated", channel: "sessions:all", id: "session-1" });
      harness.emit({ type: "course.updated", channel: "courses:all", id: "course-1" });
    });
    await act(async () => vi.advanceTimersByTimeAsync(100));

    expect(onEvent.mock.calls.map(([event]) => event.channel)).toEqual(["sessions:all", "courses:all"]);
  });

  it("coalesces duplicate keys to the latest event", async () => {
    vi.useFakeTimers();
    const harness = createHarness();
    const onEvent = vi.fn();
    renderHook(() => useRealtime(["sessions:all"], onEvent, { debounceMs: 100 }), { wrapper: harness.wrapper });

    act(() => {
      harness.emit({ type: "session.updated", channel: "sessions:all", id: "session-1", payload: { version: 1 } });
      harness.emit({ type: "session.updated", channel: "sessions:all", id: "session-1", payload: { version: 2 } });
    });
    await act(async () => vi.advanceTimersByTimeAsync(100));

    expect(onEvent).toHaveBeenCalledTimes(1);
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({ payload: { version: 2 } }));
  });

  it("delivers immediately when debounce is disabled", () => {
    const harness = createHarness();
    const onEvent = vi.fn();
    renderHook(() => useRealtime(["sessions:all"], onEvent), { wrapper: harness.wrapper });

    act(() => harness.emit({ type: "session.updated", channel: "sessions:all", id: "session-1" }));

    expect(onEvent).toHaveBeenCalledTimes(1);
  });

  it("discards pending events after unmount", async () => {
    vi.useFakeTimers();
    const harness = createHarness();
    const onEvent = vi.fn();
    const { unmount } = renderHook(() => useRealtime(["sessions:all"], onEvent, { debounceMs: 100 }), {
      wrapper: harness.wrapper,
    });

    act(() => harness.emit({ type: "session.updated", channel: "sessions:all", id: "session-1" }));
    unmount();
    await act(async () => vi.advanceTimersByTimeAsync(100));

    expect(onEvent).not.toHaveBeenCalled();
  });

  it("invokes the latest recovery callback after reconnect", () => {
    const harness = createHarness();
    const first = vi.fn();
    const second = vi.fn();
    const { rerender } = renderHook(
      ({ onReconnect }) => useRealtime(["sessions:all"], vi.fn(), { onReconnect }),
      { initialProps: { onReconnect: first }, wrapper: harness.wrapper },
    );

    rerender({ onReconnect: second });
    act(() => harness.reconnect());

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
  });

  it("starts invoking recovery when a callback is added after mount", () => {
    const harness = createHarness();
    const callback = vi.fn();
    const { rerender } = renderHook(
      ({ onReconnect }: { onReconnect?: () => void }) => useRealtime(["sessions:all"], vi.fn(), { onReconnect }),
      {
        initialProps: { onReconnect: undefined } as { onReconnect?: () => void },
        wrapper: harness.wrapper,
      },
    );

    rerender({ onReconnect: callback });
    act(() => harness.reconnect());

    expect(callback).toHaveBeenCalledTimes(1);
  });
});
