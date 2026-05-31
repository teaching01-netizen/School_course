import { useCallback, useEffect, useRef, useState } from "react";

export type WizardDirection = "forward" | "back";

export function useWizard(initialStep = 0) {
  const [step, setStep] = useState(initialStep);
  const [direction, setDirection] = useState<WizardDirection>("forward");
  const [isTransitioning, setIsTransitioning] = useState(false);
  const scrollResetRef = useRef<number | null>(null);

  const goTo = useCallback((nextStep: number) => {
    setDirection(nextStep >= step ? "forward" : "back");
    setStep(nextStep);
    setIsTransitioning(true);
  }, [step]);

  const next = useCallback((maxStep: number) => {
    goTo(Math.min(maxStep, step + 1));
  }, [goTo, step]);

  const back = useCallback(() => {
    goTo(Math.max(0, step - 1));
  }, [goTo, step]);

  useEffect(() => {
    if (scrollResetRef.current) {
      window.clearTimeout(scrollResetRef.current);
    }
    scrollResetRef.current = window.setTimeout(() => {
      window.scrollTo({ top: 0, behavior: "instant" as ScrollBehavior });
    }, 0);
    return () => {
      if (scrollResetRef.current) {
        window.clearTimeout(scrollResetRef.current);
      }
    };
  }, [step]);

  useEffect(() => {
    return () => {
      if (scrollResetRef.current) {
        window.clearTimeout(scrollResetRef.current);
      }
    };
  }, []);

  return {
    step,
    direction,
    isTransitioning,
    setIsTransitioning,
    goTo,
    next,
    back,
  };
}
