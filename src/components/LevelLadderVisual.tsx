import { ArrowUp, ArrowDown } from "lucide-react";

type LevelLadderVisualProps = {
  minLevelForSitLower: number;
};

const LEVEL_COUNT = 4;

export function LevelLadderVisual({ minLevelForSitLower }: LevelLadderVisualProps) {
  const levels = Array.from({ length: LEVEL_COUNT }, (_, i) => i + 1);

  return (
    <div className="rounded-sm border border-gray-200 bg-white p-3">
      <div className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2">
        Sit-In Level Map
      </div>
      <div className="flex flex-col items-center gap-0">
        {levels.slice().reverse().map((level, idx) => {
          const isTop = level === LEVEL_COUNT;
          const isBottom = level === 1;
          const sitsInHigher = !isTop && level >= minLevelForSitLower;
          const sitsInLower = isTop && minLevelForSitLower < level;

          return (
            <div key={level} className="flex flex-col items-center">
              <div className="flex items-center gap-2">
                <div
                  className={`w-24 rounded-sm px-3 py-1.5 text-center text-sm font-medium border ${
                    isBottom
                      ? "bg-amber-50 border-amber-200 text-amber-800"
                      : "bg-gray-50 border-gray-200 text-gray-700"
                  }`}
                >
                  Level {level}
                </div>
                {isBottom && (
                  <span className="text-xs px-2 py-0.5 rounded-full bg-blue-100 text-blue-700">
                    Zoom
                  </span>
                )}
                {!isBottom && (
                  <span className="text-xs text-gray-500">
                    {isTop && sitsInLower ? (
                      <span className="flex items-center gap-0.5 text-orange-600">
                        <ArrowDown className="w-3 h-3" />
                        sits in Level {level - 1} (lower)
                      </span>
                    ) : sitsInHigher ? (
                      <span className="flex items-center gap-0.5 text-blue-600">
                        <ArrowUp className="w-3 h-3" />
                        sits in Level {level + 1}
                      </span>
                    ) : (
                      <span className="text-gray-400">no sit-in</span>
                    )}
                  </span>
                )}
              </div>
              {idx < levels.length - 1 && (
                <div className="h-3 w-px bg-gray-300" />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

export default LevelLadderVisual;
