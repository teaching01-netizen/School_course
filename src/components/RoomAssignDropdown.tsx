import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, Loader2, X } from "lucide-react";

export type RoomAssignRoom = { id: string; name: string; capacity: number | null };

interface RoomAssignDropdownProps {
  /** Currently assigned room id, or null when the session has no room. */
  value: string | null;
  /** Called with the room id to assign, or null to clear. */
  onCommit: (roomId: string | null) => void;
  rooms: RoomAssignRoom[];
  /** roomId → reason the room cannot be selected (e.g. "Busy 09:00–10:30"). */
  busy?: Map<string, string>;
  /** Row-level pending save: trigger disabled and spinner shown. */
  disabled?: boolean;
  saving?: boolean;
  placeholder?: string;
}

export default function RoomAssignDropdown({
  value,
  onCommit,
  rooms,
  busy,
  disabled = false,
  saving = false,
  placeholder = "Assign room",
}: RoomAssignDropdownProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [highlightIndex, setHighlightIndex] = useState(-1);
  const [placement, setPlacement] = useState<"bottom" | "top">("bottom");
  const containerRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const uid = useId();
  const listboxId = `${uid}-room-listbox`;

  const selected = useMemo(() => rooms.find((r) => r.id === value) ?? null, [rooms, value]);
  const busyLabel = useCallback(
    (roomId: string) => (busy ? busy.get(roomId) : undefined),
    [busy],
  );

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return rooms;
    return rooms.filter((r) => r.name.toLowerCase().includes(q));
  }, [query, rooms]);

  const hasNoResults = open && filtered.length === 0;

  const close = useCallback(() => {
    setOpen(false);
    setHighlightIndex(-1);
  }, []);

  const commit = useCallback(
    (room: RoomAssignRoom) => {
      if (disabled || busyLabel(room.id)) return;
      onCommit(room.id === value ? null : room.id);
    },
    [busyLabel, disabled, onCommit, value],
  );

  const clear = useCallback(() => {
    if (disabled || saving) return;
    onCommit(null);
  }, [disabled, onCommit, saving]);

  useEffect(() => {
    if (!open) return;
    inputRef.current?.focus();
  }, [open]);

  // Notion-style anchored popover: flip above the trigger when there is not
  // enough room below the viewport edge.
  useLayoutEffect(() => {
    if (!open) return;
    const trigger = triggerRef.current;
    const panel = panelRef.current;
    if (!trigger || !panel) return;
    const rect = trigger.getBoundingClientRect();
    const spaceBelow = window.innerHeight - rect.bottom;
    const spaceAbove = rect.top;
    if (spaceBelow < panel.offsetHeight + 16 && spaceAbove > panel.offsetHeight + 16) {
      setPlacement("top");
    } else {
      setPlacement("bottom");
    }
  }, [open]);

  useEffect(() => {
    setHighlightIndex(-1);
  }, [query]);

  useEffect(() => {
    if (!open) return;
    const onDocMouseDown = (e: MouseEvent) => {
      const el = containerRef.current;
      if (!el) return;
      if (e.target instanceof Node && !el.contains(e.target)) close();
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        close();
        triggerRef.current?.focus();
      }
    };
    document.addEventListener("mousedown", onDocMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onDocMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [close, open]);

  const handleTriggerClick = () => {
    if (disabled || saving) return;
    if (open) close();
    else {
      setQuery("");
      setOpen(true);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case "ArrowDown": {
        e.preventDefault();
        if (filtered.length === 0) return;
        setHighlightIndex((prev) => (prev < filtered.length - 1 ? prev + 1 : 0));
        break;
      }
      case "ArrowUp": {
        e.preventDefault();
        if (filtered.length === 0) return;
        setHighlightIndex((prev) => (prev > 0 ? prev - 1 : filtered.length - 1));
        break;
      }
      case "Enter":
      case " ":
      case "Spacebar": {
        e.preventDefault();
        if (highlightIndex >= 0 && highlightIndex < filtered.length) {
          commit(filtered[highlightIndex]);
        }
        break;
      }
      case "Escape": {
        close();
        triggerRef.current?.focus();
        break;
      }
    }
  };

  const activeOptionId =
    highlightIndex >= 0 && highlightIndex < filtered.length
      ? `assign-room-option-${filtered[highlightIndex].id}`
      : undefined;

  return (
    <div ref={containerRef} className="relative inline-flex">
      <button
        ref={triggerRef}
        type="button"
        onClick={handleTriggerClick}
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listboxId}
        aria-busy={saving || undefined}
        title={selected?.name}
        className="flex h-8 w-56 items-center justify-between gap-2 rounded-sm border var(--color-wi-line) bg-white px-2.5 text-[13px] text-left transition-[border-color] duration-150 hover:border-[var(--color-wi-text-light)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15 disabled:cursor-not-allowed disabled:bg-wi-bg disabled:opacity-60 motion-reduce:transition-none"
      >
        <span className={`min-w-0 flex-1 truncate ${selected ? "" : "text-[var(--color-wi-text-light)]"}`}>
          {selected ? selected.name : placeholder}
        </span>
        {saving ? (
          <Loader2 size={14} className="shrink-0 animate-spin text-[var(--color-wi-text-light)]" aria-hidden="true" />
        ) : (
          <ChevronDown size={14} className={`shrink-0 text-[var(--color-wi-faint)] transition-transform duration-150 motion-reduce:transition-none ${open ? "rotate-180" : ""}`} aria-hidden="true" />
        )}
      </button>
      {selected && !disabled && !saving && (
        <button
          type="button"
          aria-label="Clear room"
          onClick={clear}
          className="absolute right-7 top-1/2 -translate-y-1/2 rounded-sm p-0.5 text-[var(--color-wi-faint)] transition-colors duration-150 hover:text-[var(--color-wi-text)] focus-visible:outline-none focus-visible:text-[var(--color-wi-text)]"
        >
          <X size={12} aria-hidden="true" />
        </button>
      )}
      {open && (
        <div
          ref={panelRef}
          className={`absolute right-0 z-20 w-max min-w-64 max-w-[28rem] rounded-sm border var(--border-strong) bg-white shadow-lg ${
            placement === "top" ? "bottom-full mb-1.5" : "top-full mt-1.5"
          }`}
        >
          <p className="px-3 pt-2 pb-1 text-xs font-semibold text-[var(--color-wi-text-light)]">
            Assign room
          </p>
          <div className="px-1.5 pb-1">
            <input
              ref={inputRef}
              role="combobox"
              aria-expanded="true"
              aria-controls={listboxId}
              aria-activedescendant={activeOptionId}
              aria-label="Search rooms"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Search rooms…"
              className="h-8 w-full rounded-sm border var(--color-wi-line) px-2.5 text-sm placeholder:text-[var(--color-wi-faint)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15"
            />
          </div>
          {hasNoResults ? (
            <div role="status" className="px-3 py-3 text-center text-[13px] text-[var(--color-wi-text-light)]">
              No rooms found
            </div>
          ) : (
            <div
              id={listboxId}
              role="listbox"
              aria-label="Rooms"
              className="max-h-60 overflow-y-auto pb-1.5"
            >
              {filtered.map((room) => {
                const busyText = busyLabel(room.id);
                const checked = room.id === value;
                return (
                  <div
                    key={room.id}
                    id={`assign-room-option-${room.id}`}
                    role="option"
                    aria-selected={checked}
                    aria-disabled={busyText ? true : undefined}
                    className={`px-1 ${highlightIndex >= 0 && filtered[highlightIndex]?.id === room.id ? "bg-[var(--color-wi-row-alt)]" : ""}`}
                  >
                    <label
                      className={`flex w-full cursor-pointer items-start gap-2.5 rounded-sm px-1.5 py-1.5 transition-colors duration-100 hover:bg-[var(--color-wi-row-alt)] ${
                        busyText ? "cursor-not-allowed opacity-60 hover:bg-transparent" : ""
                      } motion-reduce:transition-none`}
                    >
                      <input
                        type="checkbox"
                        className="sr-only"
                        checked={checked}
                        disabled={Boolean(busyText)}
                        onChange={() => commit(room)}
                      />
                      <span
                        aria-hidden="true"
                        className="mt-px flex h-[15px] w-[15px] shrink-0 items-center justify-center rounded-[3px] border transition-[background-color,border-color,transform] duration-100 motion-reduce:transition-none"
                        style={{
                          backgroundColor: checked ? "var(--color-wi-primary)" : undefined,
                          borderColor: checked ? "var(--color-wi-primary)" : "var(--color-wi-line)",
                          transform: checked ? "scale(1)" : "scale(0.95)",
                        }}
                      >
                        {checked && <Check size={11} strokeWidth={3} className="text-white" />}
                      </span>
                      <span className="min-w-0">
                        <span className={`block break-words text-[13px] leading-tight ${busyText ? "text-[var(--color-wi-text-light)]" : "text-[var(--color-wi-text)]"}`}>
                          {room.name}
                        </span>
                        {busyText ? (
                          <span className="block text-[11px] text-[var(--color-wi-red)]">{busyText}</span>
                        ) : room.capacity != null ? (
                          <span className="block text-[11px] text-[var(--color-wi-faint)]">{room.capacity} seats</span>
                        ) : null}
                      </span>
                    </label>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}
    </div>
  );
}