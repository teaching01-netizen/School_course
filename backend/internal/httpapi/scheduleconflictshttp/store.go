package scheduleconflictshttp

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
)

type conflictStore struct {
	db *pgxpool.Pool
}

func (s conflictStore) list(ctx context.Context, filters listFilters) (listResponse, error) {
	query, args := pageQuery(filters)
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return listResponse{}, fmt.Errorf("query schedule conflict page: %w", err)
	}
	defer rows.Close()

	items := make([]conflictDTO, 0, filters.Limit+1)
	itemIndexes := make(map[string]int, filters.Limit+1)
	for rows.Next() {
		item, student, err := scanConflict(rows)
		if err != nil {
			return listResponse{}, err
		}
		if index, ok := itemIndexes[item.ID]; ok {
			if student != nil {
				items[index].AffectedStudents = append(items[index].AffectedStudents, *student)
			}
			continue
		}
		if student != nil {
			item.AffectedStudents = []studentDTO{*student}
		}
		itemIndexes[item.ID] = len(items)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return listResponse{}, fmt.Errorf("iterate schedule conflict page: %w", err)
	}

	hasExtra := len(items) > filters.Limit
	if hasExtra {
		items = items[:filters.Limit]
	}
	if filters.Cursor != nil && filters.Cursor.Direction == cursorPrev {
		slices.Reverse(items)
	}

	response := listResponse{Items: items, Limit: filters.Limit}
	response.HasPrev = filters.Cursor != nil && (filters.Cursor.Direction == cursorNext || hasExtra)
	response.HasNext = hasExtra || (filters.Cursor != nil && filters.Cursor.Direction == cursorPrev)
	if len(items) == 0 {
		return response, nil
	}
	if response.HasNext {
		cursor, err := cursorFor(items[len(items)-1], cursorNext)
		if err != nil {
			return listResponse{}, err
		}
		response.NextCursor = &cursor
	}
	if response.HasPrev {
		cursor, err := cursorFor(items[0], cursorPrev)
		if err != nil {
			return listResponse{}, err
		}
		response.PrevCursor = &cursor
	}
	return response, nil
}

func (s conflictStore) summary(ctx context.Context, filters listFilters) (summaryDTO, error) {
	query, args := summaryQuery(filters)
	var result summaryDTO
	if err := s.db.QueryRow(ctx, query, args...).Scan(
		&result.TotalConflicts,
		&result.RoomOverlaps,
		&result.TeacherOverlaps,
		&result.StudentOverlaps,
	); err != nil {
		return summaryDTO{}, fmt.Errorf("query schedule conflict summary: %w", err)
	}
	return result, nil
}
