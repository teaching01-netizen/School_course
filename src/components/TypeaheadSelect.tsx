import { useEffect, useMemo, useRef, useState, useCallback, useId } from "react";

export type TypeaheadOption = { value: string; label: string; keywords?: string };

export default function TypeaheadSelect(props: {
  id?: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
  value: string;
  onChange: (value: string) => void;
  options: TypeaheadOption[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}) {
  const { value, onChange, options, placeholder, disabled, className } = props;
  const { id, "aria-invalid": ariaInvalid, "aria-describedby": ariaDescribedBy } = props;
  const selected = useMemo(() => options.find((o) => o.value === value) ?? null, [options, value]);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [highlightIndex, setHighlightIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const uid = useId();
  const listboxId = `${uid}-listbox`;
  const getOptionId = (index: number) => `${uid}-option-${index}`;

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
        id={id}
        role="combobox"
        aria-expanded={open}
        aria-controls={listboxId}
        aria-activedescendant={highlightIndex >= 0 ? getOptionId(highlightIndex) : undefined}
        aria-autocomplete="list"
        aria-invalid={ariaInvalid}
        aria-describedby={ariaDescribedBy}
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
        className="w-full rounded-sm border border-[var(--color-wi-line)] px-2.5 py-1.5 text-sm transition-colors duration-150 placeholder:text-[var(--color-wi-faint)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15"
      />
      {open && !disabled && (
        <div
          id={listboxId}
          role="listbox"
          className="notion-scrollbar animate-notion-popover-in absolute z-20 mt-1 max-h-64 w-full overflow-auto rounded-md border border-[var(--border-strong)] bg-white p-1 shadow-[0_12px_32px_rgba(0,0,0,0.14),0_1px_3px_rgba(0,0,0,0.12)]"
        >
          {hasNoResults ? (
            <div className="px-3 py-2 text-sm text-[var(--color-wi-faint)]">No matches found</div>
          ) : (
            filtered.map((o, i) => (
              <button
                key={o.value}
                id={getOptionId(i)}
                role="option"
                aria-selected={o.value === value}
                type="button"
                onClick={() => {
                  onChange(o.value);
                  setOpen(false);
                }}
                onMouseEnter={() => setHighlightIndex(i)}
                className={`w-full rounded-[4px] px-2 py-1.5 text-left text-sm transition-colors duration-150 ${
                  i === highlightIndex ? "bg-[var(--color-wi-selected)]" : "hover:bg-[var(--color-wi-row-alt)]"
                } ${o.value === value ? "bg-[var(--color-wi-row-alt)]" : ""}`}
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
