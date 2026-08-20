import WILogo from "../WILogo";

interface WorkspaceTileProps {
  size?: number;
}

/** Navy brand tile used at the top of the sidebar and next to the user identity. */
export default function WorkspaceTile({ size = 20 }: WorkspaceTileProps) {
  return (
    <span
      className="inline-flex shrink-0 items-center justify-center rounded-sm bg-[var(--color-wi-nav)]"
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      <WILogo variant="mono" showWordmark={false} size={Math.round(size * 0.85)} />
    </span>
  );
}