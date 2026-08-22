import {
  Children,
  forwardRef,
  isValidElement,
  useCallback,
  useId,
  useMemo,
  useRef,
  useState,
  type ChangeEventHandler,
  type FocusEventHandler,
  type KeyboardEventHandler,
  type MouseEventHandler,
  type ReactElement,
  type ReactNode,
  type Ref,
  type SelectHTMLAttributes,
} from "react";
import { Check, Search } from "lucide-react";
import { cn } from "@/utils/cn";
import { Popover } from "./Popover";

export type SearchableSelectOption = {
  value: string;
  label: string;
  keywords?: string;
  description?: string;
  disabled?: boolean;
  group?: string;
};

export type SearchableSelectSize = "sm" | "md";

export interface SearchableSelectProps
  extends Omit<SelectHTMLAttributes<HTMLSelectElement>, "children" | "onChange" | "onBlur" | "onFocus" | "onKeyDown" | "onMouseDown" | "size" | "value"> {
  value?: string | number;
  onChange?: ChangeEventHandler<HTMLSelectElement>;
  onValueChange?: (value: string) => void;
  onBlur?: FocusEventHandler<HTMLElement>;
  onFocus?: FocusEventHandler<HTMLElement>;
  onKeyDown?: KeyboardEventHandler<HTMLElement>;
  onMouseDown?: MouseEventHandler<HTMLElement>;
  options?: SearchableSelectOption[];
  children?: ReactNode;
  placeholder?: string;
  searchPlaceholder?: string;
  size?: SearchableSelectSize;
  error?: boolean;
  describedBy?: string;
  triggerMode?: "native" | "input";
}

type OptionElementProps = {
  value?: string;
  label?: string;
  disabled?: boolean;
  children?: ReactNode;
};

type OptionGroupElementProps = {
  label?: string;
  children?: ReactNode;
};

function textFromNode(node: ReactNode): string {
  if (node == null || typeof node === "boolean") return "";
  if (typeof node === "string" || typeof node === "number" || typeof node === "bigint") return String(node);
  if (Array.isArray(node)) return node.map(textFromNode).join("");
  if (isValidElement<{ children?: ReactNode }>(node)) return textFromNode(node.props.children);
  return "";
}

function optionFromElement(element: ReactElement<OptionElementProps>, group?: string): SearchableSelectOption {
  const label = element.props.label ?? textFromNode(element.props.children);
  return {
    value: element.props.value ?? label,
    label,
    disabled: element.props.disabled,
    group,
  };
}

function optionsFromChildren(children: ReactNode): SearchableSelectOption[] {
  const options: SearchableSelectOption[] = [];
  Children.forEach(children, (child) => {
    if (!isValidElement<OptionElementProps | OptionGroupElementProps>(child)) return;
    if (child.type === "option") {
      options.push(optionFromElement(child as ReactElement<OptionElementProps>));
      return;
    }
    if (child.type === "optgroup") {
      const groupProps = child.props as OptionGroupElementProps;
      const group = textFromNode(groupProps.label);
      Children.forEach(groupProps.children, (groupChild) => {
        if (isValidElement<OptionElementProps>(groupChild) && groupChild.type === "option") {
          options.push(optionFromElement(groupChild, group));
        }
      });
    }
  });
  return options;
}

function optionChildren(options: SearchableSelectOption[]): ReactNode {
  return options.map((option) => (
    <option key={option.value} value={option.value} disabled={option.disabled}>
      {option.label}
    </option>
  ));
}

function assignRef<T>(ref: Ref<T> | undefined, value: T | null) {
  if (typeof ref === "function") ref(value);
  else if (ref) ref.current = value;
}

const sizeClasses: Record<SearchableSelectSize, string> = {
  sm: "px-2 py-1 text-sm",
  md: "px-2.5 py-1.5 text-sm",
};

const SearchableSelect = forwardRef<HTMLSelectElement, SearchableSelectProps>(function SearchableSelect(
  {
    value = "",
    onChange,
    onValueChange,
    onBlur,
    onFocus,
    onKeyDown,
    onMouseDown,
    options: providedOptions,
    children,
    placeholder,
    searchPlaceholder = "Search options…",
    size = "md",
    error,
    describedBy,
    triggerMode = "native",
    className,
    disabled,
    ...selectProps
  },
  forwardedRef,
) {
  const options = useMemo(
    () => providedOptions ?? optionsFromChildren(children),
    [children, providedOptions],
  );
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [highlightIndex, setHighlightIndex] = useState(-1);
  const selectRef = useRef<HTMLSelectElement | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const uid = useId();
  const listboxId = `${uid}-listbox`;
  const currentValue = String(value);
  const selected = useMemo(() => options.find((option) => option.value === currentValue) ?? null, [currentValue, options]);
  const isInputTrigger = triggerMode === "input";

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return options;
    return options.filter((option) =>
      `${option.label} ${option.value} ${option.keywords ?? ""} ${option.group ?? ""} ${option.description ?? ""}`
        .toLowerCase()
        .includes(needle),
    );
  }, [options, query]);

  const setOpenAndReset = useCallback((next: boolean) => {
    setOpen(next);
    setQuery("");
    setHighlightIndex(-1);
  }, []);

  const setSelectRef = useCallback(
    (node: HTMLSelectElement | null) => {
      selectRef.current = node;
      assignRef(forwardedRef, node);
    },
    [forwardedRef],
  );

  const emitNativeChange = useCallback((nextValue: string) => {
    const select = selectRef.current;
    if (!select) return;
    const setter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(select), "value")?.set;
    if (setter) setter.call(select, nextValue);
    else select.value = nextValue;
    select.dispatchEvent(new Event("change", { bubbles: true }));
  }, []);

  const selectValue = useCallback(
    (nextValue: string) => {
      if (disabled) return;
      if (!isInputTrigger) emitNativeChange(nextValue);
      onValueChange?.(nextValue);
      setOpenAndReset(false);
    },
    [disabled, emitNativeChange, isInputTrigger, onValueChange, setOpenAndReset],
  );

  const moveHighlight = useCallback(
    (direction: 1 | -1) => {
      const enabled = filtered.reduce<number[]>((indexes, option, index) => {
        if (!option.disabled) indexes.push(index);
        return indexes;
      }, []);
      if (enabled.length === 0) return;
      setHighlightIndex((current) => {
        const currentPosition = enabled.indexOf(current);
        const nextPosition = currentPosition < 0
          ? direction === 1 ? 0 : enabled.length - 1
          : (currentPosition + direction + enabled.length) % enabled.length;
        return enabled[nextPosition];
      });
    },
    [filtered],
  );

  const handleSearchKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLInputElement>) => {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        moveHighlight(1);
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        moveHighlight(-1);
      } else if (event.key === "Enter") {
        event.preventDefault();
        const option = filtered[highlightIndex];
        if (option && !option.disabled) selectValue(option.value);
      } else if (event.key === "Escape") {
        event.preventDefault();
        if (!isInputTrigger) {
          setOpenAndReset(false);
          selectRef.current?.focus();
        }
      }
    },
    [filtered, highlightIndex, isInputTrigger, moveHighlight, selectValue, setOpenAndReset],
  );

  const handleNativeMouseDown = useCallback(
    (event: React.MouseEvent<HTMLSelectElement>) => {
      onMouseDown?.(event);
      if (event.defaultPrevented || disabled) return;
      event.preventDefault();
    },
    [disabled, onMouseDown],
  );

  const handleNativeKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLSelectElement>) => {
      onKeyDown?.(event);
      if (event.defaultPrevented || disabled) return;
      if (event.key === "Enter" || event.key === " " || event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        setOpenAndReset(true);
      }
    },
    [disabled, onKeyDown, setOpenAndReset],
  );

  const handleNativeClick = useCallback(
    (event: React.MouseEvent<HTMLSelectElement>) => {
      selectProps.onClick?.(event);
      if (event.defaultPrevented || disabled) return;
      event.preventDefault();
      setOpenAndReset(true);
    },
    [disabled, selectProps.onClick, setOpenAndReset],
  );

  const triggerClassName = cn(
    "w-full cursor-pointer rounded-sm border transition-[background-color,border-color,box-shadow,color] duration-150 appearance-none bg-no-repeat pr-8 text-left hover:border-[var(--color-wi-text-light)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15 disabled:cursor-not-allowed disabled:bg-wi-bg disabled:opacity-60 motion-reduce:transition-none select-chevron",
    sizeClasses[size],
    error ? "border-[var(--color-wi-red)] focus:border-[var(--color-wi-red)] focus:ring-[var(--color-wi-red)]/15" : "border-wi-line",
    className,
  );

  const trigger = isInputTrigger ? (
    <input
      ref={inputRef}
      id={selectProps.id}
      role="combobox"
      aria-expanded={open}
      aria-controls={listboxId}
      aria-autocomplete="list"
      aria-invalid={error}
      aria-describedby={describedBy ?? selectProps["aria-describedby"]}
      aria-label={selectProps["aria-label"]}
      value={open ? query : selected?.label ?? ""}
      onFocus={(event) => {
        setOpenAndReset(true);
        onFocus?.(event);
      }}
      onChange={(event) => {
        setQuery(event.target.value);
        setOpen(true);
      }}
      onClick={(event) => event.preventDefault()}
      onBlur={(event) => {
        const exact = options.find((option) => option.label.trim().toLowerCase() === query.trim().toLowerCase() || option.value.trim().toLowerCase() === query.trim().toLowerCase());
        if (exact && !exact.disabled) selectValue(exact.value);
        onBlur?.(event);
      }}
      onKeyDown={handleSearchKeyDown}
      placeholder={placeholder}
      disabled={disabled}
      className={cn(
        "w-full rounded-sm border px-2.5 py-1.5 text-sm transition-colors duration-150 placeholder:text-[var(--color-wi-faint)] focus-visible:outline-none focus:border-[var(--color-wi-primary)] focus:ring-3 focus:ring-[var(--color-wi-primary)]/15",
        error ? "border-[var(--color-wi-red)]" : "border-wi-line",
        className,
      )}
    />
  ) : (
    <select
      {...selectProps}
      ref={setSelectRef}
      value={currentValue}
      disabled={disabled}
      onChange={onChange}
      onBlur={onBlur}
      onFocus={onFocus}
      onMouseDown={handleNativeMouseDown}
      onKeyDown={handleNativeKeyDown}
      onClick={handleNativeClick}
      aria-invalid={error}
      aria-describedby={describedBy ?? selectProps["aria-describedby"]}
      className={triggerClassName}
    >
      {children ?? optionChildren(options)}
    </select>
  );

  return (
    <Popover
      trigger={trigger as ReactElement<{ ref?: Ref<HTMLElement>; onClick?: React.MouseEventHandler }>}
      open={open}
      onOpenChange={setOpenAndReset}
      autoFocus={!isInputTrigger}
      closeParentOnEscape={isInputTrigger}
      role="dialog"
      ariaLabel={searchPlaceholder}
      contentClassName="w-[min(22rem,calc(100vw-1rem))] max-w-[calc(100vw-1rem)] p-2"
    >
      <div className="flex min-w-0 flex-col gap-2">
        {!isInputTrigger && (
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-wi-faint)]" aria-hidden="true" />
            <input
              aria-label={searchPlaceholder}
              value={query}
              onChange={(event) => {
                setQuery(event.target.value);
                setHighlightIndex(-1);
              }}
              onKeyDown={handleSearchKeyDown}
              placeholder={searchPlaceholder}
              className="h-9 w-full rounded-sm border border-wi-line bg-[var(--color-wi-row-alt)] pl-8 pr-2.5 text-sm placeholder:text-[var(--color-wi-faint)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-3 focus:ring-[var(--color-wi-primary)]/15"
            />
          </div>
        )}
        <div id={listboxId} role="listbox" aria-label={searchPlaceholder} className="notion-scrollbar max-h-64 overflow-y-auto">
          {filtered.length === 0 ? (
            <div className="px-2.5 py-3 text-center text-sm text-[var(--color-wi-text-light)]">No matches found</div>
          ) : (
            filtered.map((option, index) => {
              const selectedOption = option.value === currentValue;
              return (
                <div key={`${option.value}-${index}`}>
                  {option.group && (index === 0 || filtered[index - 1]?.group !== option.group) ? (
                    <div className="px-2.5 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-wi-faint)]">{option.group}</div>
                  ) : null}
                  <button
                    id={`${listboxId}-option-${index}`}
                    type="button"
                    role="option"
                    aria-selected={selectedOption}
                    aria-disabled={option.disabled || undefined}
                    disabled={option.disabled}
                    onMouseEnter={() => setHighlightIndex(index)}
                    onClick={() => selectValue(option.value)}
                    className={cn(
                      "flex min-h-9 w-full items-center gap-2 rounded-sm px-2.5 py-1.5 text-left text-sm transition-colors duration-100 focus-visible:outline-none motion-reduce:transition-none",
                      option.disabled
                        ? "cursor-not-allowed text-[var(--color-wi-faint)]"
                        : "text-[var(--color-wi-text)] hover:bg-[var(--color-wi-row-alt)] focus-visible:bg-[var(--color-wi-row-alt)]",
                      index === highlightIndex && !option.disabled ? "bg-[var(--color-wi-row-alt)]" : "",
                    )}
                  >
                    <span className="flex h-4 w-4 shrink-0 items-center justify-center">
                      {selectedOption ? <Check className="h-3.5 w-3.5 text-[var(--color-wi-primary)]" strokeWidth={2.5} aria-hidden="true" /> : null}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{option.label}</span>
                      {option.description ? <span className="mt-0.5 block truncate text-xs text-[var(--color-wi-text-light)]">{option.description}</span> : null}
                    </span>
                  </button>
                </div>
              );
            })
          )}
        </div>
      </div>
    </Popover>
  );
});

SearchableSelect.displayName = "SearchableSelect";

export default SearchableSelect;
