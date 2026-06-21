import { createContext, useCallback, useEffect, useMemo, useRef, type ReactNode } from "react";
import type { RealtimeEvent } from "@/hooks/useRealtime";

type EventHandler = (event: RealtimeEvent<any>) => void;

type RealtimeContextValue = {
  subscribe: (channel: string, handler: EventHandler) => () => void;
  subscribeReconnect: (handler: () => void) => () => void;
};

export const RealtimeContext = createContext<RealtimeContextValue | null>(null);

function realtimeURL(): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/api/v1/ws`;
}

export function reconnectDelay(attempt: number, random: () => number = Math.random): number {
  const ceiling = Math.min(2 ** Math.max(0, attempt - 1) * 1_000, 30_000);
  const floor = ceiling / 2;
  const fraction = Math.min(1, Math.max(0, random()));
  return Math.round(floor + (ceiling - floor) * fraction);
}

export function RealtimeProvider({
  enabled,
  onReconnect,
  children,
}: {
  enabled: boolean;
  onReconnect?: () => void;
  children: ReactNode;
}) {
  const socketRef = useRef<WebSocket | null>(null);
  const handlersRef = useRef(new Map<string, Set<EventHandler>>());
  const reconnectHandlersRef = useRef(new Set<() => void>());
  const onReconnectRef = useRef(onReconnect);

  useEffect(() => {
    onReconnectRef.current = onReconnect;
  }, [onReconnect]);

  const sendSubscription = useCallback((type: "subscribe" | "unsubscribe", channel: string) => {
    const socket = socketRef.current;
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ type, channel }));
    }
  }, []);

  const subscribe = useCallback((channel: string, handler: EventHandler) => {
    const handlers = handlersRef.current.get(channel) ?? new Set<EventHandler>();
    const wasEmpty = handlers.size === 0;
    handlers.add(handler);
    handlersRef.current.set(channel, handlers);
    if (wasEmpty) sendSubscription("subscribe", channel);

    return () => {
      const current = handlersRef.current.get(channel);
      if (!current) return;
      current.delete(handler);
      if (current.size === 0) {
        handlersRef.current.delete(channel);
        sendSubscription("unsubscribe", channel);
      }
    };
  }, [sendSubscription]);

  const subscribeReconnect = useCallback((handler: () => void) => {
    reconnectHandlersRef.current.add(handler);
    return () => reconnectHandlersRef.current.delete(handler);
  }, []);

  useEffect(() => {
    if (!enabled || typeof WebSocket === "undefined") return;

    let disposed = false;
    let reconnectTimer: number | null = null;
    let reconnectAttempt = 0;
    let hasConnected = false;
    let hasDisconnected = false;

    const connect = () => {
      if (disposed) return;
      const socket = new WebSocket(realtimeURL());
      socketRef.current = socket;

      socket.addEventListener("open", () => {
        if (disposed || socketRef.current !== socket) return;
        reconnectAttempt = 0;
        for (const channel of handlersRef.current.keys()) {
          socket.send(JSON.stringify({ type: "subscribe", channel }));
        }
        if (hasConnected || hasDisconnected) {
          onReconnectRef.current?.();
          for (const handler of reconnectHandlersRef.current) handler();
        }
        hasConnected = true;
      });

      socket.addEventListener("message", (message) => {
        try {
          const event = JSON.parse(message.data) as RealtimeEvent;
          if (!event || typeof event.channel !== "string" || typeof event.type !== "string") return;
          for (const handler of handlersRef.current.get(event.channel) ?? []) handler(event);
        } catch {
          // HTTP refetch on reconnect/focus remains authoritative.
        }
      });

      socket.addEventListener("close", () => {
        if (disposed || socketRef.current !== socket) return;
        hasDisconnected = true;
        reconnectAttempt += 1;
        const delay = reconnectDelay(reconnectAttempt);
        reconnectTimer = window.setTimeout(connect, delay);
      });

      socket.addEventListener("error", () => socket.close());
    };

    connect();
    return () => {
      disposed = true;
      if (reconnectTimer != null) window.clearTimeout(reconnectTimer);
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [enabled]);

  const value = useMemo<RealtimeContextValue>(() => ({ subscribe, subscribeReconnect }), [subscribe, subscribeReconnect]);
  return <RealtimeContext.Provider value={value}>{children}</RealtimeContext.Provider>;
}
