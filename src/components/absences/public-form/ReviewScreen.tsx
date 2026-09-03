export type ReviewSection = {
  key: string;
  title: string;
  lines: string[];
  onEdit?: () => void;
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
        <div role="status" className="mt-5 rounded-xl border border-[var(--color-wi-amber)]/30 bg-[var(--color-wi-amber-bg)] px-4 py-3 text-[15px] leading-snug text-[var(--color-wi-amber)]">
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
                  className="min-h-11 rounded-lg px-2 text-[13px] font-semibold text-[var(--color-wi-primary)] transition-colors motion-reduce:transition-none hover:bg-[var(--color-wi-primary)]/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  Edit {section.title.toLowerCase()}
                </button>
              ) : null}
            </div>
            <div className="mt-2 space-y-1.5">
              {section.lines.map((line, lineIndex) => (
                <p key={lineIndex} className="text-[15px] leading-relaxed text-[var(--color-wi-text)]">
                  {line}
                </p>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}