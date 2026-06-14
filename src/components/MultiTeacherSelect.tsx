import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import type { TypeaheadOption } from "./TypeaheadSelect";

export default function MultiTeacherSelect(props: {
  value: string[];
  onChange: (ids: string[]) => void;
  options: TypeaheadOption[];
  placeholder?: string;
  disabled?: boolean;
}) {
  const { value, onChange, options, placeholder, disabled } = props;
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [highlightIndex, setHighlightIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);

  const selectedSet = useMemo(() => new Set(value), [value]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    let candidates = options.filter((o) => !selectedSet.has(o.value));
    if (!q) return candidates.slice(0, 20);
    return candidates
      .filter((o) => (o.label + " " + (o.keywords ?? "")).toLowerCase().includes(q))
      .slice(0, 20);
  }, [options, query, selectedSet]);

  const hasNoResults = open && query.trim().length > 0 && filtered.length === 0;

  const add = useCallback((id: string) => {
    if (!selectedSet.has(id)) {
      onChange([...value, id]);
    }
    setQuery("");
    setOpen(false);
    setHighlightIndex(-1);
    inputRef.current?.focus();
  }, [onChange, selectedSet, value]);

  const remove = useCallback((id: string) => {
    onChange(value.filter((v) => v !== id));
  }, [onChange, value]);

  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      const el = containerRef.current;
      if (!el) return;
      if (e.target instanceof Node && !el.contains(e.target)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, []);

  useEffect(() => {
    setHighlightIndex(-1);
  }, [query]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (!open) {
      if (e.key === "ArrowDown" || e.key === "Enter") {
        setOpen(true);
        setQuery("");
        e.preventDefault();
      }
      return;
    }
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightIndex((prev) => (prev < filtered.length - 1 ? prev + 1 : 0));
        break;
      case "ArrowUp":
        e.preventDefault();
        setHighlightIndex((prev) => (prev > 0 ? prev - 1 : filtered.length - 1));
        break;
      case "Enter":
        e.preventDefault();
        if (highlightIndex >= 0 && highlightIndex < filtered.length) {
          add(filtered[highlightIndex].value);
        }
        break;
      case "Escape":
        e.preventDefault();
        setOpen(false);
        break;
    }
  }, [open, filtered, highlightIndex, add]);

  const labelById = useMemo(() => {
    const m = new Map(options.map((o) => [o.value, o.label]));
    return (id: string) => m.get(id) ?? id;
  }, [options]);

  return (
    <div ref={containerRef} className="relative">
      <div className="flex flex-wrap gap-1.5 mb-1.5">
        {value.map((id) => (
          <span key={id} className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-sm bg-blue-50 text-blue-700 border border-blue-200">
            {labelById(id)}
            <button
              type="button"
              onClick={() => remove(id)}
              className="ml-0.5 hover:text-blue-900 focus:outline-none"
              aria-label={`Remove ${labelById(id)}`}
            >
              ×
            </button>
          </span>
        ))}
      </div>
      <input
        ref={inputRef}
        role="combobox"
        aria-expanded={open}
        aria-autocomplete="list"
        value={query}
        onFocus={() => { if (disabled) return; setQuery(""); setOpen(true); }}
        onChange={(e) => { setQuery(e.target.value); setOpen(true); }}
        onKeyDown={handleKeyDown}
        placeholder={value.length === 0 ? (placeholder ?? "Select teachers…") : ""}
        disabled={disabled}
        className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
      />
      {open && !disabled && (
        <div
          role="listbox"
          className="absolute z-20 mt-1 w-full max-h-64 overflow-auto border border-gray-200 bg-white rounded-sm shadow animate-dropdown-enter"
        >
          {hasNoResults ? (
            <div className="px-3 py-2 text-sm text-gray-400">No matches found</div>
          ) : (
            filtered.map((o, i) => (
              <button
                key={o.value}
                role="option"
                aria-selected={selectedSet.has(o.value)}
                type="button"
                onClick={() => add(o.value)}
                onMouseEnter={() => setHighlightIndex(i)}
                className={`w-full text-left px-2 py-2 text-sm ${
                  i === highlightIndex ? "bg-blue-100" : "hover:bg-gray-50"
                } ${selectedSet.has(o.value) ? "bg-gray-50" : ""}`}
              >
                {o.label}
              </button>
            ))
          )}
        </div>
      )}
    </div>
  );
}
