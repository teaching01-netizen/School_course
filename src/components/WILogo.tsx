interface WILogoProps {
  className?: string;
  variant?: "color" | "mono";
  showWordmark?: boolean;
  size?: number;
}

const monoStroke = "#ffffff";

export default function WILogo({ className = "", variant = "color", showWordmark = true, size = 40 }: WILogoProps) {
  const red = variant === "mono" ? monoStroke : "var(--color-wi-red)";
  const blue = variant === "mono" ? monoStroke : "var(--color-wi-primary)";
  const gray = variant === "mono" ? "rgba(255,255,255,0.72)" : "var(--color-wi-text-light)";

  return (
    <div className={`flex items-center gap-1 ${className}`}>
      <svg width={size} height={size} viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
        <path d="M5 8L12 32L18 14L24 32L30 14L35 8" stroke={red} strokeWidth="3" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
        <path d="M8 8L15 32" stroke={blue} strokeWidth="2.5" fill="none" strokeLinecap="round"/>
        <path d="M20 8L24 20" stroke={gray} strokeWidth="2" fill="none" strokeLinecap="round"/>
      </svg>
      {showWordmark && (
        <div className="leading-tight">
          <div className={`text-[11px] font-bold tracking-wider ${variant === "mono" ? "text-white" : "text-[var(--color-wi-text)]"}`}>WARWICK</div>
          <div className={`text-[9px] tracking-[0.2em] ${variant === "mono" ? "text-white/60" : "text-[var(--color-wi-text-light)]"}`}>INSTITUTE</div>
        </div>
      )}
    </div>
  );
}