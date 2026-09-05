type ConfirmStudentScreenProps = {
  nameHint?: string;
  wcode: string;
  onYes: () => void;
  onNo: () => void;
};

export default function ConfirmStudentScreen({
  nameHint,
  wcode,
  onYes,
  onNo,
}: ConfirmStudentScreenProps) {
  return (
    <div className="mx-auto w-full max-w-2xl">
      <h1 className="text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
        Is this you?
      </h1>

      <div className="mt-8 rounded-2xl border border-[var(--color-wi-border)] bg-white px-6 py-7">
        {nameHint ? (
          <p className="text-[22px] font-semibold text-[var(--color-wi-text)]">{nameHint}</p>
        ) : null}
        <p className="mt-1 font-mono text-[17px] text-[var(--color-wi-text-light)]">{wcode}</p>
      </div>

      <p className="mt-4 text-[15px] leading-relaxed text-[var(--color-wi-text-light)]">
        Next we&apos;ll confirm with a parent by text, then pick your classes.
      </p>

      <button
        type="button"
        onClick={onYes}
        className="wi-press mt-8 flex h-[52px] w-full items-center justify-center rounded-xl bg-[var(--color-wi-primary)] px-5 text-[17px] font-semibold text-white hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2"
      >
        Yes, continue
      </button>

      <button
        type="button"
        onClick={onNo}
        className="wi-press mx-auto mt-4 block rounded-lg px-3 py-2 text-[15px] font-medium text-[var(--color-wi-text-light)] hover:bg-[var(--color-wi-row-alt)] hover:text-[var(--color-wi-text)] active:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
      >
        Not me
      </button>
    </div>
  );
}