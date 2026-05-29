import { useEffect, useMemo, useRef, useState, useCallback } from "react";

export type TypeaheadOption = { value: string; label: string; keywords?: string };

export default function TypeaheadSelect(props: {
  value: string;
  onChange: (value: string) => void;
  options: TypeaheadOption[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}) {
  const { value, onChange, options, placeholder, disabled, className } = props;
  const selected = useMemo(() => options.find((o) => o.value === value) ?? null, [options, value]);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [highlightIndex, setHighlightIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const listboxId = "typeahead-listbox";

  const commitExactQueryMatch = useCallback(() => {
    const q = query.trim().toLowerCase();
    if (!q) return;
    const match = options.find((o) => o.label.trim().toLowerCase() === q || o.value.trim().toLowerCase() === q);
    if (match && match.value !== value) onChange(match.value);
  }, [onChange, options, query, value]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options.slice(0, 20);
    const out = options
      .filter((o) => (o.label + " " + (o.keywords ?? "")).toLowerCase().includes(q))
      .slice(0, 20);
    return out;
  }, [options, query]);

  const hasNoResults = open && query.trim().length > 0 && filtered.length === 0;

  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      const el = containerRef.current;
      if (!el) return;
      if (e.target instanceof Node && !el.contains(e.target)) {
        commitExactQueryMatch();
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [commitExactQueryMatch]);

  useEffect(() => {
    setHighlightIndex(-1);
  }, [query]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
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
            onChange(filtered[highlightIndex].value);
            setOpen(false);
          }
          break;
        case "Escape":
          e.preventDefault();
          setOpen(false);
          break;
      }
    },
    [open, filtered, highlightIndex, onChange]
  );

  return (
    <div ref={containerRef} className={`relative ${className ?? ""}`}>
      <input
        ref={inputRef}
        role="combobox"
        aria-expanded={open}
        aria-controls={listboxId}
        aria-activedescendant={highlightIndex >= 0 ? `option-${filtered[highlightIndex]?.value}` : undefined}
        aria-autocomplete="list"
        value={open ? query : selected?.label ?? ""}
        onFocus={() => {
          if (disabled) return;
          setQuery("");
          setOpen(true);
        }}
        onChange={(e) => {
          setQuery(e.target.value);
          setOpen(true);
        }}
        onKeyDown={handleKeyDown}
        onBlur={() => {
          commitExactQueryMatch();
        }}
        placeholder={placeholder}
        disabled={disabled}
        className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-sm"
      />
      {open && !disabled && (
        <div
          id={listboxId}
          role="listbox"
          className="absolute z-20 mt-1 w-full max-h-64 overflow-auto border border-gray-200 bg-white rounded-sm shadow animate-dropdown-enter"
        >
          {hasNoResults ? (
            <div className="px-3 py-2 text-sm text-gray-400">No matches found</div>
          ) : (
            filtered.map((o, i) => (
              <button
                key={o.value}
                id={`option-${o.value}`}
                role="option"
                aria-selected={o.value === value}
                type="button"
                onClick={() => {
                  onChange(o.value);
                  setOpen(false);
                }}
                onMouseEnter={() => setHighlightIndex(i)}
                className={`w-full text-left px-2 py-2 text-sm ${
                  i === highlightIndex ? "bg-blue-100" : "hover:bg-gray-50"
                } ${o.value === value ? "bg-gray-50" : ""}`}
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
