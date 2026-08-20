import type { ReactNode } from "react";

interface PageHeadingProps {
  children: ReactNode;
  className?: string;
}

export default function PageHeading({ children, className = "" }: PageHeadingProps) {
  return (
    <h1 className={`text-[32px] font-semibold tracking-[-0.01em] text-[var(--color-wi-text)] ${className}`}>
      {children}
    </h1>
  );
}
