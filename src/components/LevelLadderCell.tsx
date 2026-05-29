import { useState, useCallback } from "react";

export type CellStatus = "active" | "vacant" | "gap" | "overlap" | "saving" | "error";

export interface LadderCellData {
  level: number;
  cycleId: string;
  courseId?: string;
  courseCode?: string;
  courseName?: string;
  roomName?: string;
  schedule?: string;
  studentCount?: number;
  status: CellStatus;
  errorMessage?: string;
}

interface LevelLadderCellProps {
  cell: LadderCellData;
  onClick?: (cell: LadderCellData) => void;
  onDropCourse?: (fromLevel: number, toLevel: number, cycleId: string) => void;
  selected?: boolean;
}

const statusStyles: Record<CellStatus, string> = {
  active: "border-gray-200 bg-white hover:border-blue-300",
  vacant: "border-dashed border-gray-300 bg-gray-50 hover:border-blue-400",
  gap: "border-red-300 bg-red-50 hover:border-red-400",
  overlap: "border-red-400 bg-red-100",
  saving: "border-blue-300 bg-blue-50 animate-pulse",
  error: "border-red-400 bg-red-100",
};

const statusIcons: Record<CellStatus, string | null> = {
  active: null,
  vacant: null,
  gap: "⚠️",
  overlap: "⚠️",
  saving: null,
  error: "✕",
};

export default function LevelLadderCell({ cell, onClick, onDropCourse, selected }: LevelLadderCellProps) {
  const [dragOver, setDragOver] = useState(false);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(true);
  }, []);

  const handleDragLeave = useCallback(() => {
    setDragOver(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragOver(false);
      const data = e.dataTransfer.getData("text/plain");
      if (data) {
        try {
          const parsed = JSON.parse(data);
          if (parsed.courseId && parsed.fromLevel !== cell.level && onDropCourse) {
            onDropCourse(parsed.fromLevel, cell.level, cell.cycleId, parsed.courseId);
          }
        } catch {
          // ignore
        }
      }
    },
    [cell.level, cell.cycleId, onDropCourse],
  );

  return (
    <div
      role="gridcell"
      aria-label={`Level ${cell.level} — ${cell.courseCode ?? "Vacant"}${cell.status === "gap" ? " (Gap)" : ""}`}
      aria-describedby={cell.status === "gap" ? `gap-desc-${cell.level}` : undefined}
      tabIndex={0}
      draggable={cell.status === "active"}
      onDragStart={(e) => {
        if (cell.courseId && cell.status === "active") {
          e.dataTransfer.setData(
            "text/plain",
            JSON.stringify({ fromLevel: cell.level, fromCycleId: cell.cycleId, courseId: cell.courseId }),
          );
        }
      }}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      onClick={() => onClick?.(cell)}
      onKeyDown={(e) => {
        if ((e.key === "Enter" || e.key === " ") && onClick) {
          e.preventDefault();
          onClick(cell);
        }
      }}
      className={`
        relative flex flex-col justify-between p-2 min-h-[80px] border rounded-sm cursor-pointer
        transition-colors duration-150 select-none
        ${statusStyles[cell.status]}
        ${dragOver ? "ring-2 ring-blue-400" : ""}
        ${selected ? "ring-2 ring-blue-500" : ""}
        focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500
      `}
    >
      {/* Level number badge */}
      <div className="absolute top-1 left-1 text-[10px] font-semibold text-gray-400">
        L{cell.level}
      </div>

      {/* Status icon */}
      {statusIcons[cell.status] && (
        <div className="absolute top-1 right-1 text-xs" role="img" aria-hidden="true">
          {statusIcons[cell.status]}
        </div>
      )}

      {/* Course info */}
      {cell.status === "active" && cell.courseCode && (
        <>
          <div className="mt-3 text-xs font-bold text-gray-800 truncate">{cell.courseCode}</div>
          {cell.schedule && <div className="text-[10px] text-gray-500 truncate">{cell.schedule}</div>}
          {cell.roomName && <div className="text-[10px] text-gray-400 truncate">📍{cell.roomName}</div>}
          {cell.studentCount !== undefined && (
            <div className="text-[10px] text-gray-400">{cell.studentCount} stud</div>
          )}
        </>
      )}

      {/* Vacant cell */}
      {cell.status === "vacant" && (
        <div className="flex items-center justify-center h-full mt-3">
          <span className="text-xs text-gray-400 font-medium">[+ Add]</span>
        </div>
      )}

      {/* Gap cell */}
      {cell.status === "gap" && (
        <div className="flex flex-col items-center justify-center h-full mt-3 gap-1">
          <span id={`gap-desc-${cell.level}`} className="text-xs text-red-600 font-medium">
            Gap
          </span>
        </div>
      )}

      {/* Overlap cell */}
      {cell.status === "overlap" && (
        <div className="flex items-center justify-center h-full mt-3">
          <span className="text-xs text-red-600 font-medium">Conflict</span>
        </div>
      )}

      {/* Saving spinner */}
      {cell.status === "saving" && (
        <div className="flex items-center justify-center h-full mt-3">
          <div className="w-4 h-4 border-2 border-blue-400 border-t-transparent rounded-full animate-spin" />
        </div>
      )}

      {/* Error state */}
      {cell.status === "error" && cell.errorMessage && (
        <div className="flex items-center justify-center h-full mt-3">
          <span className="text-[10px] text-red-600 truncate">{cell.errorMessage}</span>
        </div>
      )}
    </div>
  );
}
