import { useEffect, useRef, useState } from "react";
import { motion, useReducedMotion } from "framer-motion";
import { LoaderCircle } from "lucide-react";
import { apiJson, ApiRequestError } from "@/api/client";
import { loadStudentProfile } from "@/features/absences/api/absenceFormApi";
import {
  clearStudentSessionHint,
  hasStudentSessionHint,
  markStudentSessionHint,
} from "@/features/absences/storage/studentResumeStorage";
import OtpInput from "./OtpInput";
import SmsSendButton from "./SmsSendButton";
import type { ParentVerificationResponse } from "@/types";

type VerificationStore = {
  code: string;
  setCode: (next: string) => void;
  token: string | null;
  persistToken: (nextToken: string, nextExpiresAt?: number | null) => void;
  clearStoredToken: () => void;
};

type StepCoverVerificationProps = {
  wcode: string;
  lookupToken: string;
  parentVerificationAvailable?: boolean;
  parentPhone?: string | null;
  online?: boolean;
  smsParentEnabled?: boolean;
  adminContact?: { email: string; phone: string; hours: string };
  verification: VerificationStore;
  completed: boolean;
  onSatisfied: () => void;
  onRestart: () => void;
  onRestored: () => void;
};

function isRetryable(err: unknown): boolean {
  if (err instanceof ApiRequestError) {
    return !err.status || err.status >= 500;
  }
  return err instanceof TypeError;
}

/** Masks every digit but the last two, grouped for readability (e.g. • ••• ••• 42). */
function maskEnrollNumber(digits: string): string {
  if (!digits) return "";
  const combined = "•".repeat(Math.max(0, digits.length - 2)) + digits.slice(-2);
  const chunks: string[] = [];
  for (let i = combined.length; i > 0; i -= 3) chunks.unshift(combined.slice(Math.max(0, i - 3), i));
  return chunks.join(" ");
}
export default function StepCoverVerification({
  wcode,
  lookupToken,
  parentVerificationAvailable,
  parentPhone,
  online = true,
  smsParentEnabled = true,
  verification,
  completed,
  onSatisfied,
  onRestart,
  onRestored,
}: StepCoverVerificationProps) {
  const verificationAvailable = parentVerificationAvailable ?? Boolean(parentPhone);
  const [session, setSession] = useState<ParentVerificationResponse | null>(null);
  const [sendError, setSendError] = useState<string | null>(null);
  const [verifyError, setVerifyError] = useState<string | null>(null);
  const [verifyRetryable, setVerifyRetryable] = useState(false);
  const [sendCount, setSendCount] = useState(0);
  const [isSending, setIsSending] = useState(false);
  const [isVerifying, setIsVerifying] = useState(false);
  const [lastSentAt, setLastSentAt] = useState<number | null>(null);
  const [restoreError, setRestoreError] = useState<string | null>(null);
  const [validationAttempt, setValidationAttempt] = useState(0);
  const [enrollPhone, setEnrollPhone] = useState("");
  const autoVerifyCodeRef = useRef<string | null>(null);
  const reduceMotion = useReducedMotion();

  const verified = completed || session?.status === "verified" || session?.status === "consumed";
  const deliveryStatus = session?.delivery_status;
  const deliveryPending = deliveryStatus === "queued"
    || deliveryStatus === "preparing"
    || deliveryStatus === "submitting"
    || deliveryStatus === "retryable";
  const deliveryAccepted = deliveryStatus === "accepted" || (!deliveryStatus && lastSentAt !== null);
  // The number lives in React state only: it is sent once to start the OTP
  // and never written to local storage.
  const enrollPhoneDigits = enrollPhone.replace(/\D/g, "");
  const enrollPhoneValid = enrollPhoneDigits.length >= 9 && enrollPhoneDigits.length <= 12;

  useEffect(() => {
    if (!verification.token || !wcode) {
      setRestoreError(null);
      return;
    }
    if (!online) {
      setRestoreError("You're offline. Reconnect to validate saved verification.");
      return;
    }
    const controller = new AbortController();
    setRestoreError(null);
    void apiJson<ParentVerificationResponse>(
      "/api/v1/absences/parent-verification/status",
      { method: "POST", body: JSON.stringify({ token: verification.token }), signal: controller.signal },
    )
      .then((response) => {
        if (controller.signal.aborted) return;
        if (response.wcode !== wcode || response.status === "consumed") {
          setSession(null);
          onRestart();
          return;
        }
        setSession(response);
        if (response.status === "verified") onRestored();
      })
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        if (error instanceof ApiRequestError && (error.status === 400 || error.status === 410)) {
          setSession(null);
          onRestart();
          return;
        }
        setRestoreError("Could not validate saved verification. Check your connection and try again.");
      });
    return () => controller.abort();
  }, [verification.token, wcode, online, validationAttempt, onRestart, onRestored]);

  useEffect(() => {
    if (
      verification.token
      || completed
      || !wcode
      || !online
      || !hasStudentSessionHint()
    ) {
      return;
    }
    let active = true;
    setRestoreError(null);
    void loadStudentProfile()
      .then((profile) => {
        if (!active) return;
        if (profile.wcode !== wcode) {
          clearStudentSessionHint();
          setRestoreError("Your verified session belongs to a different Student ID. Verify this Student ID again.");
          onRestart();
          return;
        }
        onRestored();
      })
      .catch((error: unknown) => {
        if (!active) return;
        if (error instanceof ApiRequestError && (error.status === 401 || error.status === 403)) {
          clearStudentSessionHint();
          return;
        }
        setRestoreError("Could not restore your verified session. Check your connection and try again.");
      });
    return () => {
      active = false;
    };
  }, [verification.token, completed, wcode, online, onRestart, onRestored]);

  useEffect(() => {
    const token = session?.token ?? verification.token;
    if (!online || !token || !deliveryPending) return;
    const controller = new AbortController();
    const poll = window.setInterval(() => {
      void apiJson<ParentVerificationResponse>(
        "/api/v1/absences/parent-verification/status",
        { method: "POST", body: JSON.stringify({ token }), signal: controller.signal },
      ).then((response) => {
        if (controller.signal.aborted) return;
        setSession(response);
        if (response.delivery_status === "accepted") setLastSentAt(Date.now());
      }).catch(() => {
        // A later poll can recover from a transient status-read failure.
      });
    }, 1000);
    return () => {
      controller.abort();
      window.clearInterval(poll);
    };
  }, [session?.token, verification.token, deliveryPending, online]);

  useEffect(() => {
    if (!online || verified || isSending || isVerifying || !verification.token) return;
    const normalized = verification.code.replace(/\D/g, "").slice(0, 6);
    if (normalized.length !== 6) { autoVerifyCodeRef.current = null; return; }
    if (autoVerifyCodeRef.current === normalized) return;
    autoVerifyCodeRef.current = normalized;
    void handleVerify();
  }, [verification.code, verification.token, verified, isSending, isVerifying, online]);

  async function handleSend(startNewSession = false) {
    // With no phone on file, sending is only possible through enrollment.
    const enrolling = !verificationAvailable;
    if (!online || !smsParentEnabled || !lookupToken) return;
    if (enrolling && !enrollPhoneValid) return;
    if (startNewSession) {
      clearStudentSessionHint();
      onRestart();
      setSession(null);
    }
    setIsSending(true);
    setSendError(null);
    setVerifyError(null);
    setVerifyRetryable(false);
    try {
      const startingNewSession = startNewSession || !verification.token;
      // A family with no phone on file enrolls theirs here; the OTP proves
      // the number is reachable before the server persists it.
      const newSessionBody: Record<string, string> = { lookup_token: lookupToken };
      if (enrolling && enrollPhone.trim()) newSessionBody.parent_phone = enrollPhone.trim();
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/send", {
        method: "POST",
        body: JSON.stringify(startingNewSession ? newSessionBody : { token: verification.token }),
      });
      setSession(response);
      verification.persistToken(response.token, response.expires_at ? Date.parse(response.expires_at) : null);
      verification.setCode("");
      if (!response.delivery_status || response.delivery_status === "accepted") {
        setLastSentAt(Date.now());
      } else {
        setLastSentAt(null);
      }
      // A delivery that definitively failed never reached the parent, so it
      // must not start the resend cooldown — the student needs to retry now.
      const deliveryFailed = response.delivery_status === "failed" || response.delivery_status === "expired";
      if (!deliveryFailed) setSendCount((c) => c + 1);
    } catch (err) {
      // A send that errored (network/HTTP) dispatched nothing; keep the button
      // immediately retryable instead of arming the 5-minute resend cooldown.
      setSendError(err instanceof Error ? err.message : "Could not send verification code");
    } finally {
      setIsSending(false);
    }
  }

  async function handleVerify() {
    if (!online || !verification.token || verification.code.length !== 6) return;
    setIsVerifying(true);
    setVerifyError(null);
    setVerifyRetryable(false);
    try {
      const response = await apiJson<ParentVerificationResponse>("/api/v1/absences/parent-verification/verify", {
        method: "POST",
        body: JSON.stringify({ token: verification.token, code: verification.code }),
      });
      setSession(response);
      markStudentSessionHint();
      verification.clearStoredToken();
      verification.setCode("");
      onSatisfied();
    } catch (err) {
      const retryable = isRetryable(err);
      // A definitive rejection from the server almost always means the code
      // was wrong; say that plainly instead of echoing a technical message.
      const wrongCode = err instanceof ApiRequestError && !retryable;
      const message = wrongCode
        ? "That code isn't right. Check the message and try again."
        : err instanceof Error ? err.message : "Verification failed";
      setVerifyError(message);
      setVerifyRetryable(retryable);
      // Keep the typed code so one wrong digit costs one digit, not six — but
      // leave autoVerifyCodeRef pointing at this code. The auto-verify effect
      // re-runs when isVerifying flips back to false, and with the ref still
      // set it declines to re-send the same unchanged code, so a rejected code
      // never loops. Editing the code drops the ref via the effect's
      // length !== 6 branch, which re-enables auto-verification for the fix.
    } finally {
      setIsVerifying(false);
    }
  }
  const parentMissing = !verificationAvailable;
  // Enrollment: once a valid number is entered, echo it back for confirmation
  // before any OTP goes out. The input stays mounted so typing is never cut
  // off by the panel appearing.
  const showEnrollConfirm = parentMissing && enrollPhoneValid && !verified;
  const canSend = online
    && smsParentEnabled
    && !isSending
    && !isVerifying
    && !verified
    && (!parentMissing || enrollPhoneValid);

  if (verified) {
    return (
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-medium text-[var(--color-wi-green-dark)]">✓ Verified</p>
        {smsParentEnabled && !parentMissing ? (
          <button
            type="button"
            onClick={() => void handleSend(true)}
            disabled={!online}
            className="min-h-6 px-1 text-xs font-semibold text-[var(--color-wi-primary)] hover:text-[var(--color-wi-primary-dark)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            Send new code
          </button>
        ) : null}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {!online ? (
        <p role="status" aria-live="polite" className="rounded-lg bg-[var(--color-wi-amber-bg)] px-3 py-2 text-sm font-medium text-[var(--color-wi-amber-ink)]">
          You're offline. Reconnect to send or verify the parent code.
        </p>
      ) : null}
      {!smsParentEnabled ? (
        <p role="alert" className="text-sm text-[var(--color-wi-amber-ink)]">
          Parent verification codes are currently unavailable.
          Contact admin before continuing.
        </p>
      ) : null}

      {sendError ? (
        <p role="alert" className="text-sm text-[var(--color-wi-red)]">{sendError}</p>
      ) : null}
      {restoreError ? (
        <div role="alert" className="space-y-1 text-sm text-[var(--color-wi-red)]">
          <p>{restoreError}</p>
          <button
            type="button"
            onClick={() => setValidationAttempt((attempt) => attempt + 1)}
            className="wi-press min-h-6 rounded px-1 font-semibold underline underline-offset-2"
          >
            Retry verification check
          </button>
        </div>
      ) : null}
      {verifyError ? (
        <div id="verify-error" className="space-y-1 text-sm text-[var(--color-wi-red)]">
          <p role="alert">{verifyError}</p>
          {verifyRetryable && verification.code.length === 6 ? (
            <button
              type="button"
              onClick={() => void handleVerify()}
              disabled={isVerifying}
              className="wi-press min-h-6 rounded px-1 font-semibold underline underline-offset-2 disabled:opacity-50"
            >
              Retry verification
            </button>
          ) : null}
        </div>
      ) : null}

      {smsParentEnabled && !parentMissing && !verified ? (
        <p className="text-xs text-[var(--color-wi-text-light)]">
          We'll send a code to your parent's phone for consent.
        </p>
      ) : null}

      {smsParentEnabled && parentMissing && !verified ? (
        <>
          <div className="space-y-1.5">
            <label htmlFor="parent-phone-input" className="block text-xs font-medium text-[var(--color-wi-text-light)]">
              Parent's phone number <span className="text-[var(--color-wi-red)]" aria-hidden="true">*</span>
            </label>
            <input
              id="parent-phone-input"
              type="tel"
              inputMode="tel"
              autoComplete="tel"
              className="min-h-[48px] w-full rounded-xl border border-[var(--color-wi-border)] bg-white px-4 text-base text-[var(--color-wi-text)] placeholder:text-[var(--color-wi-text-light)] focus:border-[var(--color-wi-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--color-wi-primary)]/20"
              placeholder="e.g. 0812345678"
              value={enrollPhone}
              onChange={(e) => setEnrollPhone(e.target.value)}
            />
            <p className="text-xs text-[var(--color-wi-text-light)]">
              We don't have your parent's phone number yet. Enter it below — we'll text a one-time code to check it works, and keep the number so you don't have to re-enter it next time.
            </p>
            {!enrollPhoneValid && enrollPhone.trim() ? (
              <p className="text-sm text-[var(--color-wi-amber-ink)]">Enter a valid phone number (9–12 digits).</p>
            ) : null}
          </div>
          {showEnrollConfirm ? (
            // The number is echoed back before any OTP goes out, so a typo is
            // recoverable with one tap instead of a failed delivery.
            <div className="rounded-xl border border-[var(--color-wi-border)] bg-white p-4">
              <p className="text-[15px] font-semibold text-[var(--color-wi-text)]">Confirm this number</p>
              <p className="mt-1 font-mono text-[20px] font-semibold tracking-widest text-[var(--color-wi-text)]">
                {maskEnrollNumber(enrollPhoneDigits)}
              </p>
              <p className="mt-1 text-xs text-[var(--color-wi-text-light)]">
                We&apos;ll send a code here.
              </p>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <SmsSendButton
                  isSending={isSending || deliveryPending}
                  sendCount={sendCount}
                  disabled={!canSend}
                  onClick={() => void handleSend()}
                />
                <button
                  type="button"
                  onClick={() => {
                    setEnrollPhone("");
                    setSendError(null);
                  }}
                  className="wi-press min-h-11 rounded-lg px-3 text-[15px] font-semibold text-[var(--color-wi-primary)] hover:bg-[var(--color-wi-row-alt)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-wi-primary)]"
                >
                  Change number
                </button>
              </div>
            </div>
          ) : null}
        </>
      ) : null}

      {smsParentEnabled && !parentMissing ? (
        <SmsSendButton
          isSending={isSending || deliveryPending}
          sendCount={sendCount}
          disabled={!canSend}
          onClick={() => void handleSend()}
        />
      ) : null}

      {deliveryPending ? (
        <p className="text-xs font-medium text-[var(--color-wi-primary)]">Sending your code…</p>
      ) : null}

      {deliveryStatus === "uncertain" ? (
        <p role="status" className="text-sm font-medium text-[var(--color-wi-amber-ink)]">
          Your code may still arrive. You can request another when the timer ends.
        </p>
      ) : null}

      {deliveryStatus === "failed" ? (
        <p role="alert" className="text-sm font-medium text-[var(--color-wi-red)]">
          We couldn't send the code. Tap Send code to try again.
        </p>
      ) : null}

      {deliveryStatus === "expired" ? (
        <p role="alert" className="text-sm font-medium text-[var(--color-wi-red)]">
          That code has expired. We&apos;ll send you a new one.
        </p>
      ) : null}

      {deliveryAccepted && lastSentAt && !isSending && (
        <motion.p
          initial={reduceMotion ? false : { opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          transition={reduceMotion ? { duration: 0 } : undefined}
          className="flex items-center gap-1.5 text-xs font-medium text-[var(--color-wi-green-dark)]"
        >
          <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
          {/* parent_phone arrives pre-masked from the server (e.g. ••••5678) */}
          Your code has been sent to {session?.parent_phone || "your parent's phone"}. It may take a moment to arrive.
        </motion.p>
      )}

      {(session?.token || verification.token) ? (
        <motion.div
          initial={reduceMotion ? false : { opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          transition={reduceMotion ? { duration: 0 } : { duration: 0.25, ease: "easeOut" }}
          className="space-y-3"
        >
          <OtpInput
            value={verification.code}
            onChange={verification.setCode}
            disabled={isSending || isVerifying}
            error={!!verifyError}
            autoFocus={sendCount > 0}
            label="Confirmation code"
            describedBy={verifyError ? "verify-error" : undefined}
          />
          <p className="text-xs text-[var(--color-wi-text-light)]">Enter the 6-digit code sent to your parent's phone.</p>
          {isVerifying ? (
            <p role="status" aria-live="polite" className="flex items-center gap-2 text-xs font-medium text-[var(--color-wi-primary)]">
              <LoaderCircle className="h-3.5 w-3.5 animate-spin motion-reduce:animate-none" aria-hidden="true" />
              Checking your code…
            </p>
          ) : null}
        </motion.div>
      ) : null}

    </div>
  );
}
