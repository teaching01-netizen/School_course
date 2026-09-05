import { motion, useReducedMotion } from "framer-motion";

export type SuccessGroup = {
  key: string;
  label: string;
  absence: string;
  makeup: string;
};

type SuccessScreenProps = {
  groups: SuccessGroup[];
  reference: string;
  onDone: () => void;
};

export default function SuccessScreen({ groups, reference, onDone }: SuccessScreenProps) {
  const reduceMotion = useReducedMotion();
  return (
    <div className="mx-auto w-full max-w-xl py-6 text-center">
      <motion.div
        initial={reduceMotion ? false : { opacity: 0, scale: 0.96 }}
        animate={{ opacity: 1, scale: 1 }}
        transition={reduceMotion ? { duration: 0 } : { duration: 0.18, ease: "easeOut" }}
        className="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-[var(--color-wi-green)]/10"
        role="status"
        aria-live="polite"
      >
        <svg className="h-8 w-8 text-[var(--color-wi-green)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5} aria-hidden="true">
          <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
        </svg>
      </motion.div>

      <h1 className="mt-6 text-[28px] font-bold leading-tight tracking-tight text-[var(--color-wi-text)]">
        Absence submitted
      </h1>
      <p className="mx-auto mt-2 max-w-sm text-[17px] leading-relaxed text-[var(--color-wi-text-light)]">
        We&apos;ll review your request and send updates to your email.
      </p>

      <div className="mt-8 space-y-4 text-left">
        {groups.map((group) => (
          <div key={group.key} className="rounded-2xl border border-[var(--color-wi-border)] bg-white px-5 py-4">
            <p className="text-[15px] font-semibold text-[var(--color-wi-text)]">{group.label}</p>
            <p className="mt-0.5 text-[13px] text-[var(--color-wi-text-light)]">
              Missed · {group.absence}
            </p>
            <p className="mt-0.5 text-[13px] text-[var(--color-wi-text-light)]">
              Make-up · {group.makeup}
            </p>
          </div>
        ))}
      </div>

      {reference ? (
        <p className="mt-6 text-[13px] leading-relaxed text-[var(--color-wi-text-light)]">
          Reference <span className="font-mono font-semibold text-[var(--color-wi-text)]">{reference}</span>
          <span className="mt-1 block">Quote this if you contact Student Services about this absence.</span>
        </p>
      ) : null}
      <p className="mt-3 text-[13px] text-[var(--color-wi-text-light)]">
        Need to report another absence? Tap Done to start a new report.
      </p>

      <button
        type="button"
        onClick={onDone}
        className="wi-press mt-8 flex h-[52px] w-full items-center justify-center rounded-xl bg-[var(--color-wi-primary)] px-5 text-[17px] font-semibold text-white hover:bg-[var(--color-wi-primary-dark)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2"
      >
        Done
      </button>
    </div>
  );
}