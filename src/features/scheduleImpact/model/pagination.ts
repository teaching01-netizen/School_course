export interface PaginationRange {
  start: number;
  end: number;
  total: number;
}

/**
 * Compute the display range for a paginated list.
 * Returns 1-based values suitable for "Showing {start}–{end} of {total}".
 */
export function computePageRange(offset: number, limit: number, total: number): PaginationRange {
  if (total === 0) return { start: 0, end: 0, total: 0 };
  const safeOffset = Math.max(0, Math.min(offset, total - 1));
  return {
    start: safeOffset + 1,
    end: Math.min(safeOffset + limit, total),
    total,
  };
}

/**
 * Normalize an offset to stay within valid bounds [0, total-1].
 */
export function normalizeOffset(offset: number, total: number): number {
  if (offset < 0) return 0;
  if (offset >= total && total > 0) return Math.max(0, total - 1);
  return offset;
}

/**
 * Compute the next page offset.
 */
export function nextOffset(currentOffset: number, limit: number): number {
  return currentOffset + limit;
}

/**
 * Compute the previous page offset, clamped to 0.
 */
export function prevOffset(currentOffset: number, limit: number): number {
  return Math.max(0, currentOffset - limit);
}
