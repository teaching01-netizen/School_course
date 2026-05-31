import { useEffect, useMemo, useRef, useState } from "react";
import clsx from "clsx";

type CountdownTimerProps = {
  secondsLeft: number;
  className?: string;
  onAvailable?: () => void;
  label?: string;
};

function formatSeconds(seconds: number): string {
  const safe = Math.max(0, seconds);
  const minutes = Math.floor(safe / 60);
  const secs = safe % 60;
  return `${String(minutes).padStart(2, "0")}:${String(secs).padStart(2, "0")}`;
}

function milestoneLabel(secondsLeft: number): string {
  if (secondsLeft <= 0) return "Resend button now available";
  if (secondsLeft === 60 || secondsLeft === 30 || secondsLeft === 10) {
    return `${secondsLeft} seconds until you can resend`;
  }
  if (secondsLeft <= 5 && secondsLeft > 0) {
    return `${secondsLeft} seconds until you can resend`;
  }
  return "";
}

export default function CountdownTimer({
  secondsLeft,
  className,
  onAvailable,
  label = "Resend available in",
}: CountdownTimerProps) {
  const prevRef = useRef<number>(secondsLeft);
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    const prev = prevRef.current;
    prevRef.current = secondsLeft;

    if (prev > 0 && secondsLeft <= 0) {
      setAnnouncement("Resend code button now available");
      onAvailable?.();
      return;
    }

    const nextAnnouncement = milestoneLabel(secondsLeft);
    if (nextAnnouncement && nextAnnouncement !== milestoneLabel(prev)) {
      setAnnouncement(nextAnnouncement);
    }
  }, [secondsLeft, onAvailable]);

  const display = useMemo(() => formatSeconds(secondsLeft), [secondsLeft]);

  return (
    <div className={clsx("space-y-1", className)}>
      <div
        role="timer"
        aria-live="off"
        className="text-xs font-medium text-gray-600 tabular-nums"
      >
        {label} {display}
      </div>
      <div role="status" aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </div>
  );
}
