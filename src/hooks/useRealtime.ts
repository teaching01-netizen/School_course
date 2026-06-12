import { useEffect, useRef } from "react";

export type RealtimeEvent<TPayload = unknown> = {
  type: string;
  channel: string;
  id?: string;
  payload?: TPayload;
};

type RealtimeOptions = {
  enabled?: boolean;
  debounceMs?: number;
};

function realtimeURL(): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/api/v1/ws`;
}

export function useRealtime<TPayload = unknown>(
  channels: string[],
  onEvent: (event: RealtimeEvent<TPayload>) => void,
  options: RealtimeOptions = {}
) {
  const enabled = options.enabled ?? true;
  const debounceMs = options.debounceMs ?? 0;
  const onEventRef = useRef(onEvent);
  const debounceRef = useRef<number | null>(null);
  const key = channels.join("|");

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    if (!enabled || channels.length === 0 || typeof WebSocket === "undefined") return;

    let closed = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: number | null = null;
    let attempt = 0;

    const clearReconnect = () => {
      if (reconnectTimer != null) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    };

    const handleEvent = (event: RealtimeEvent<TPayload>) => {
      if (debounceMs <= 0) {
        onEventRef.current(event);
        return;
      }
      if (debounceRef.current != null) window.clearTimeout(debounceRef.current);
      debounceRef.current = window.setTimeout(() => {
        debounceRef.current = null;
        onEventRef.current(event);
      }, debounceMs);
    };

    const connect = () => {
      socket = new WebSocket(realtimeURL());

      socket.addEventListener("open", () => {
        attempt = 0;
        for (const channel of channels) {
          socket?.send(JSON.stringify({ type: "subscribe", channel }));
        }
      });

      socket.addEventListener("message", (message) => {
        try {
          handleEvent(JSON.parse(message.data) as RealtimeEvent<TPayload>);
        } catch {
          // Ignore malformed realtime messages; the next HTTP fetch remains authoritative.
        }
      });

      socket.addEventListener("close", () => {
        if (closed) return;
        attempt += 1;
        const delay = Math.min(2 ** Math.max(0, attempt - 1) * 1000, 30_000);
        reconnectTimer = window.setTimeout(connect, delay);
      });

      socket.addEventListener("error", () => {
        socket?.close();
      });
    };

    connect();

    return () => {
      closed = true;
      clearReconnect();
      if (debounceRef.current != null) {
        window.clearTimeout(debounceRef.current);
        debounceRef.current = null;
      }
      if (socket && socket.readyState === WebSocket.OPEN) {
        for (const channel of channels) {
          socket.send(JSON.stringify({ type: "unsubscribe", channel }));
        }
      }
      socket?.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, debounceMs, key]);
}
