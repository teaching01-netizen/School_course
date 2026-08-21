import { useEffect, useRef, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
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
        className="transition-[stroke-dashoffset] duration-500 ease-linear"
      />
    </svg>
  );
}

function Spinner() {
  return (
    <svg
      className="h-5 w-5 animate-spin"
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
  const [cooldown, setCooldown] = useState(0);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const expiryRef = useRef<number | null>(null);
  const cooldownKeyRef = useRef(0);

  useEffect(() => {
    if (!isSending && sendCount > 0 && cooldown === 0) {
      cooldownKeyRef.current += 1;
      const key = cooldownKeyRef.current;
      const expiry = Date.now() + cooldownDuration * 1000;
      expiryRef.current = expiry;
      setCooldown(cooldownDuration);

      const schedule = () => {
        if (cooldownKeyRef.current !== key) return;
        if (timeoutRef.current) clearTimeout(timeoutRef.current);
        const remaining = expiryRef.current != null ? Math.max(0, Math.ceil((expiryRef.current - Date.now()) / 1000)) : 0;
        if (remaining <= 0) {
          setCooldown(0);
          expiryRef.current = null;
          timeoutRef.current = null;
          return;
        }
        timeoutRef.current = setTimeout(() => {
          if (cooldownKeyRef.current !== key) return;
          const nextRemaining = expiryRef.current != null ? Math.max(0, Math.ceil((expiryRef.current - Date.now()) / 1000)) : 0;
          if (nextRemaining <= 0) {
            setCooldown(0);
            expiryRef.current = null;
            timeoutRef.current = null;
          } else {
            setCooldown(nextRemaining);
            schedule();
          }
        }, 1000);
      };
      schedule();

      const onVisibility = () => {
        if (!document.hidden && cooldownKeyRef.current === key && expiryRef.current != null) {
          const remaining = Math.max(0, Math.ceil((expiryRef.current - Date.now()) / 1000));
          setCooldown(remaining);
          if (remaining <= 0) {
            if (timeoutRef.current) clearTimeout(timeoutRef.current);
            expiryRef.current = null;
            timeoutRef.current = null;
          } else {
            schedule();
          }
        }
      };
      document.addEventListener("visibilitychange", onVisibility);
      return () => {
        document.removeEventListener("visibilitychange", onVisibility);
        if (timeoutRef.current && cooldownKeyRef.current === key) {
          clearTimeout(timeoutRef.current);
          timeoutRef.current = null;
        }
      };
    }
  }, [isSending, sendCount, cooldownDuration, cooldown]);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  }, []);

  if (parentPhoneMissing) return null;

  const onCooldown = cooldown > 0;
  const isInteractive = !isSending && !onCooldown && !disabled;
  const progress = cooldownDuration > 0 ? cooldown / cooldownDuration : 0;

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!isInteractive}
      className={clsx(
        "relative inline-flex min-h-[44px] min-w-[140px] items-center justify-center gap-2 rounded-lg px-4 text-sm font-semibold transition-all",
        isInteractive
          ? "bg-blue-600 text-white hover:bg-blue-700"
          : "cursor-not-allowed bg-[var(--color-wi-row-alt)] text-[var(--color-wi-text-light)]",
      )}
    >
      <AnimatePresence mode="wait">
        {isSending ? (
          <motion.span
            key="sending"
            initial={{ opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.9 }}
            transition={{ duration: 0.15 }}
            className="inline-flex items-center gap-2"
          >
            <Spinner />
            <span>Sending...</span>
          </motion.span>
        ) : onCooldown ? (
          <motion.span
            key="cooldown"
            initial={{ opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.9 }}
            transition={{ duration: 0.15 }}
            className="inline-flex items-center gap-2"
          >
            <ProgressRing progress={progress} />
            <span className="tabular-nums">Resend in {cooldown}s</span>
          </motion.span>
        ) : (
          <motion.span
            key={sendCount > 0 ? "resend" : "send"}
            initial={{ opacity: 0, scale: 0.9 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.9 }}
            transition={{ duration: 0.15 }}
            className={clsx(
              "inline-flex items-center gap-2",
              sendCount > 0 && "animate-pulse",
            )}
          >
            {sendCount > 0 ? "Resend code" : "Send code"}
          </motion.span>
        )}
      </AnimatePresence>
    </button>
  );
}
