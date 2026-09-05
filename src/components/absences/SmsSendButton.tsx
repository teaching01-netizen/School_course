import { useEffect, useRef, useState } from "react";
import { motion, AnimatePresence, useReducedMotion } from "framer-motion";
import clsx from "clsx";

type SmsSendButtonProps = {
  isSending: boolean;
  sendCount: number;
  disabled?: boolean;
  onClick: () => void;
  cooldownDuration?: number;
  parentPhoneMissing?: boolean;
};

function ProgressRing({ progress }: { progress: number }) {
  const radius = 14;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference * (1 - Math.min(progress, 1));

  return (
    <svg
      className="h-8 w-8 -rotate-90"
      viewBox="0 0 32 32"
      aria-hidden="true"
    >
      <circle
        cx="16"
        cy="16"
        r={radius}
        fill="none"
        stroke="currentColor"
        strokeWidth="3"
        className="opacity-20"
      />
      <circle
        cx="16"
        cy="16"
        r={radius}
        fill="none"
        stroke="currentColor"
        strokeWidth="3"
        strokeLinecap="round"
        strokeDasharray={circumference}
        strokeDashoffset={offset}
        className="transition-[stroke-dashoffset] duration-500 ease-linear motion-reduce:transition-none"
      />
    </svg>
  );
}

function Spinner() {
  return (
    <svg
      className="h-5 w-5 animate-spin motion-reduce:animate-none"
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      />
    </svg>
  );
}

export default function SmsSendButton({
  isSending,
  sendCount,
  disabled = false,
  onClick,
  cooldownDuration = 5 * 60,
  parentPhoneMissing = false,
}: SmsSendButtonProps) {
  const reduceMotion = useReducedMotion();
  const [cooldown, setCooldown] = useState(0);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const cooldownKeyRef = useRef(0);

  useEffect(() => {
    if (!isSending && sendCount > 0 && cooldown === 0) {
      cooldownKeyRef.current += 1;
      const key = cooldownKeyRef.current;
      setCooldown(cooldownDuration);

      intervalRef.current = setInterval(() => {
        setCooldown((prev) => {
          if (prev <= 1) {
            if (intervalRef.current) clearInterval(intervalRef.current);
            intervalRef.current = null;
            return 0;
          }
          return prev - 1;
        });
      }, 1000);

      return () => {
        if (intervalRef.current && cooldownKeyRef.current === key) {
          clearInterval(intervalRef.current);
          intervalRef.current = null;
        }
      };
    }
  }, [isSending, sendCount, cooldownDuration]);

  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, []);

  if (parentPhoneMissing) return null;

  const onCooldown = cooldown > 0;
  const isInteractive = !isSending && !onCooldown && !disabled;
  const progress = cooldownDuration > 0 ? cooldown / cooldownDuration : 0;
  const cooldownLabel = onCooldown
    ? `Send another code in ${Math.floor(cooldown / 60)}:${String(cooldown % 60).padStart(2, "0")}`
    : sendCount > 0 ? "Resend code" : "Send code";

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!isInteractive}
      aria-label={onCooldown ? cooldownLabel : undefined}
      className={clsx(
        "wi-press relative inline-flex min-h-[44px] min-w-[140px] items-center justify-center gap-2 rounded-lg px-4 text-sm font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)] focus-visible:ring-offset-2",
        isInteractive
          ? "bg-[var(--color-wi-primary)] text-white hover:bg-[var(--color-wi-primary-dark)]"
          : "cursor-not-allowed bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]",
      )}
    >
      <AnimatePresence mode="wait">
        {isSending ? (
          <motion.span
            key="sending"
            initial={reduceMotion ? false : { opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={reduceMotion ? { opacity: 1, scale: 1 } : { opacity: 0, scale: 0.9 }}
            transition={{ duration: reduceMotion ? 0 : 0.15 }}
            className="inline-flex items-center gap-2"
          >
            <Spinner />
            <span>Sending...</span>
          </motion.span>
        ) : onCooldown ? (
          <motion.span
            key="cooldown"
            initial={reduceMotion ? false : { opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={reduceMotion ? { opacity: 1, scale: 1 } : { opacity: 0, scale: 0.9 }}
            transition={{ duration: reduceMotion ? 0 : 0.15 }}
            className="inline-flex items-center gap-2"
          >
            <ProgressRing progress={progress} />
            <span className="tabular-nums" aria-hidden="true">Send another code in {Math.floor(cooldown / 60)}:{String(cooldown % 60).padStart(2, "0")}</span>
          </motion.span>
        ) : (
          <motion.span
            key={sendCount > 0 ? "resend" : "send"}
            initial={reduceMotion ? false : { opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={reduceMotion ? { opacity: 1, scale: 1 } : { opacity: 0, scale: 0.9 }}
            transition={{ duration: reduceMotion ? 0 : 0.15 }}
            className="inline-flex items-center gap-2"
          >
            {sendCount > 0 ? "Resend code" : "Send code"}
          </motion.span>
        )}
      </AnimatePresence>
    </button>
  );
}
