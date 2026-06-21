import { useContext, useEffect, useRef } from "react";
import { RealtimeContext } from "@/realtime/RealtimeProvider";

export type RealtimeEvent<TPayload = unknown> = {
  type: string;
  channel: string;
  id?: string;
  payload?: TPayload;
};

type RealtimeOptions = {
  enabled?: boolean;
  debounceMs?: number;
  onReconnect?: () => void;
};

export function useRealtime<TPayload = unknown>(
  channels: string[],
  onEvent: (event: RealtimeEvent<TPayload>) => void,
  options: RealtimeOptions = {}
) {
  const enabled = options.enabled ?? true;
  const debounceMs = options.debounceMs ?? 0;
  const onReconnect = options.onReconnect;
  const realtime = useContext(RealtimeContext);
  const onEventRef = useRef(onEvent);
  const onReconnectRef = useRef(onReconnect);
  const debounceRef = useRef<number | null>(null);
  const pendingEventsRef = useRef(new Map<string, RealtimeEvent<TPayload>>());
  const key = channels.join("|");

  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    onReconnectRef.current = onReconnect;
  }, [onReconnect]);

  useEffect(() => {
    if (!enabled || channels.length === 0 || realtime == null) return;

    const handleEvent = (event: RealtimeEvent<TPayload>) => {
      if (debounceMs <= 0) {
        onEventRef.current(event);
        return;
      }
      const eventKey = `${event.channel}\u0000${event.id ?? event.type}`;
      pendingEventsRef.current.set(eventKey, event);
      if (debounceRef.current != null) window.clearTimeout(debounceRef.current);
      debounceRef.current = window.setTimeout(() => {
        debounceRef.current = null;
        const events = [...pendingEventsRef.current.values()];
        pendingEventsRef.current.clear();
        for (const pendingEvent of events) onEventRef.current(pendingEvent);
      }, debounceMs);
    };

    const unsubscribers = channels.map((channel) => realtime.subscribe(channel, handleEvent));
    const unsubscribeReconnect = realtime.subscribeReconnect(() => onReconnectRef.current?.());

    return () => {
      for (const unsubscribe of unsubscribers) unsubscribe();
      unsubscribeReconnect();
      if (debounceRef.current != null) {
        window.clearTimeout(debounceRef.current);
        debounceRef.current = null;
      }
      pendingEventsRef.current.clear();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, debounceMs, key, realtime]);
}
