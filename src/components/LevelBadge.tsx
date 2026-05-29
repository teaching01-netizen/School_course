type LevelBadgeProps = {
  level: number | null;
};

export default function LevelBadge({ level }: LevelBadgeProps) {
  if (level === null) {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-500">
        — Not set
      </span>
    );
  }

  if (level === 1) {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-700">
        🎥 Zoom
      </span>
    );
  }

  return (
    <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-700">
      ✓ Eligible
    </span>
  );
}
