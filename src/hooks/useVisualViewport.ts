import { useEffect, useState } from "react";

export type VisualViewportState = {
  height: number;
  offsetTop: number;
  keyboardLikelyOpen: boolean;
};

export function useVisualViewport(): VisualViewportState {
  const [viewport, setViewport] = useState<VisualViewportState>(() => {
    const visualViewport = typeof window !== "undefined" ? window.visualViewport : undefined;
    const height = visualViewport?.height ?? (typeof window !== "undefined" ? window.innerHeight : 0);
    const offsetTop = visualViewport?.offsetTop ?? 0;
    return {
      height,
      offsetTop,
      keyboardLikelyOpen: typeof window !== "undefined" && height < window.innerHeight * 0.9,
    };
  });

  useEffect(() => {
    const visualViewport = window.visualViewport;
    const update = () => {
      const height = visualViewport?.height ?? window.innerHeight;
      const offsetTop = visualViewport?.offsetTop ?? 0;
      setViewport({
        height,
        offsetTop,
        keyboardLikelyOpen: height < window.innerHeight * 0.9 || offsetTop > 0,
      });
    };

    visualViewport?.addEventListener("resize", update);
    visualViewport?.addEventListener("scroll", update);
    window.addEventListener("resize", update);
    update();

    return () => {
      visualViewport?.removeEventListener("resize", update);
      visualViewport?.removeEventListener("scroll", update);
      window.removeEventListener("resize", update);
    };
  }, []);

  return viewport;
}
