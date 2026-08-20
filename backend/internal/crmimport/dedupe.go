package crmimport

import (
	"sort"

	"warwick-institute/internal/crmimport/xlsx"
)

func deduplicateRows(rows []xlsx.Row) []xlsx.Row {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].OrderQuoteUpdatedAt, rows[j].OrderQuoteUpdatedAt
		if a == nil && b == nil {
			return false
		}
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return a.After(*b)
	})

	seen := make(map[string]struct{}, len(rows))
	deduped := make([]xlsx.Row, 0, len(rows))
	for _, row := range rows {
		hash := row.Hash()
		if _, exists := seen[hash]; exists {
			continue
		}
		seen[hash] = struct{}{}
		deduped = append(deduped, row)
	}
	return deduped
}
