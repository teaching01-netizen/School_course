import { Fragment } from "react";
import LevelLadderCell from "./LevelLadderCell";
import type { LadderCellData } from "./LevelLadderCell";

interface LevelLadderCanvasProps {
  cycles: Array<{ cycleId: string; cycleLabel: string }>;
  levels: number[];
  getCell: (level: number, cycleId: string) => LadderCellData;
  onCellClick?: (cell: LadderCellData) => void;
  onCellDrop?: (fromLevel: number, toLevel: number, cycleId: string, courseId: string) => void;
  selectedCell?: { level: number; cycleId: string } | null;
  onSelectCell?: (cell: { level: number; cycleId: string } | null) => void;
}

export default function LevelLadderCanvas({
  cycles,
  levels,
  getCell,
  onCellClick,
  onCellDrop,
  selectedCell,
}: LevelLadderCanvasProps) {
  if (cycles.length === 0) {
    return (
      <div className="flex items-center justify-center h-32 text-sm text-gray-400">
        No cycles configured for this subject.
      </div>
    );
  }

  if (levels.length === 0) {
    return (
      <div className="flex items-center justify-center h-32 text-sm text-gray-400">
        No level data available.
      </div>
    );
  }

  return (
    <div
      role="grid"
      aria-label="Level ladder grid — cycles as columns, levels as rows"
      className="overflow-x-auto"
    >
      <div
        className="grid gap-2 min-w-[600px]"
        style={{
          gridTemplateColumns: `80px repeat(${cycles.length}, 1fr)`,
        }}
      >
        {/* Header row: cycle labels */}
        <div className="text-[10px] font-semibold text-gray-500 uppercase tracking-wide px-2 py-1 self-end">
          Level
        </div>
        {cycles.map((cycle) => (
          <div
            key={cycle.cycleId}
            className="text-[10px] font-semibold text-gray-500 uppercase tracking-wide px-2 py-1 text-center truncate"
            title={cycle.cycleLabel}
          >
            {cycle.cycleLabel}
          </div>
        ))}

        {/* Level rows */}
        {levels.map((level) => (
          <Fragment key={`row-${level}`}>
            {/* Level label column */}
            <div className="flex items-center justify-end pr-2 text-xs font-medium text-gray-400 h-[80px]">
              L{level}
            </div>

            {/* Cell per cycle */}
            {cycles.map((cycle) => {
              const cell = getCell(level, cycle.cycleId);
              const isSelected =
                selectedCell?.level === level && selectedCell?.cycleId === cycle.cycleId;
              return (
                <LevelLadderCell
                  key={`${cycle.cycleId}-${level}`}
                  cell={cell}
                  onClick={onCellClick}
                  onDropCourse={onCellDrop}
                  selected={isSelected}
                />
              );
            })}
          </Fragment>
        ))}
      </div>
    </div>
  );
}
