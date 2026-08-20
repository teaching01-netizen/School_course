const ROW_GRID = "grid grid-cols-[minmax(0,5.5rem)_minmax(0,1fr)] items-center gap-x-2";

/** Notion property-row grammar: label on the left, the editor fills the row. */
export function PropertyRow({ label, htmlFor, children }: { label: string; htmlFor: string; children: React.ReactNode }) {
  return (
    <div className={`${ROW_GRID} rounded-[4px] px-1.5 py-1 transition-colors duration-150 hover:bg-[var(--color-wi-row-alt)] motion-reduce:transition-none`}>
      <label htmlFor={htmlFor} className="truncate text-[13px] text-[var(--color-wi-text-light)]">
        {label}
      </label>
      {children}
    </div>
  );
}