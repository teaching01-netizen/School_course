export type ReviewSection = {
  key: string;
  title: string;
  lines: string[];
  onEdit?: () => void;
  /** Optional per-line edit (parallel to lines); rendered under structured lines. */
  onEditLine?: (lineIndex: number) => void;
  editLineLabel?: string;
};

type ReviewScreenProps = {
  studentName: string;
  wcode: string;
  sections: ReviewSection[];
  notice?: string | null;
};

export default function ReviewScreen({ studentName, wcode, sections, notice = null }: ReviewScreenProps) {
  return (
    <div className="mx-auto w-full max-w-xl">
      <h1 className="text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
        Review your absence
      </h1>
      <p className="mt-2 text-[15px] text-[var(--color-wi-text-light)]">
        {studentName} · <span className="font-mono">{wcode}</span>
      </p>

      {notice ? (
        <div role="alert" className="mt-5 rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-4 py-3 text-[15px] leading-snug text-[var(--color-wi-amber)]">
          {notice}
        </div>
      ) : null}

      <div className="mt-7 overflow-hidden rounded-2xl border border-[var(--color-wi-border)] bg-white">
        {sections.map((section, index) => (
          <div
            key={section.key}
            className={index > 0 ? "border-t border-[var(--color-wi-line)] px-5 py-4" : "px-5 py-4"}
          >
            <div className="flex items-center justify-between gap-4">
              <h2 className="text-[13px] font-semibold uppercase tracking-[0.08em] text-[var(--color-wi-text-light)]">
                {section.title}
              </h2>
              {section.onEdit ? (
                <button
                  type="button"
                  onClick={section.onEdit}
                  className="wi-press min-h-11 rounded-lg px-2 text-[13px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  Edit {section.title.toLowerCase()}
                </button>
              ) : null}
            </div>
            <div className="mt-2 space-y-3">
              {section.lines.map((line, lineIndex) => {
                const makeupSplit = line.split(" — Make-up: ");
                if (makeupSplit.length === 2) {
                  return (
                    <div key={lineIndex} className="rounded-xl bg-[var(--color-wi-bg)] px-3 py-2">
                      <p className="text-[15px] font-medium leading-relaxed text-[var(--color-wi-text)]">
                        {makeupSplit[0]}
                      </p>
                      <p className="mt-0.5 text-[13px] leading-relaxed text-[var(--color-wi-text-light)]">
                        Make-up: {makeupSplit[1]}
                      </p>
                      {section.onEditLine ? (
                        <button
                          type="button"
                          onClick={() => section.onEditLine?.(lineIndex)}
                          className="wi-press mt-1 min-h-9 rounded-lg px-2 text-[13px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary)]/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                        >
                          {section.editLineLabel ?? "Edit make-up"}
                        </button>
                      ) : null}
                    </div>
                  );
                }
                return (
                  <p key={lineIndex} className="text-[15px] leading-relaxed text-[var(--color-wi-text)]">
                    {line}
                  </p>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}