export default function WILogo({ className = '' }: { className?: string }) {
  return (
    <div className={`flex items-center gap-1 ${className}`}>
      <svg width="40" height="40" viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path d="M5 8L12 32L18 14L24 32L30 14L35 8" stroke="#c0392b" strokeWidth="3" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
        <path d="M8 8L15 32" stroke="#2980b9" strokeWidth="2.5" fill="none" strokeLinecap="round"/>
        <path d="M20 8L24 20" stroke="#7f8c8d" strokeWidth="2" fill="none" strokeLinecap="round"/>
      </svg>
      <div className="leading-tight">
        <div className="text-[11px] font-bold tracking-wider text-gray-800">WARWICK</div>
        <div className="text-[9px] tracking-[0.2em] text-gray-500">INSTITUTE</div>
      </div>
    </div>
  );
}
