package absenceshttp

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

func normalizeSubmissionSitInMethod(raw *string) (pgtype.Text, error) {
	if raw == nil {
		return pgtype.Text{}, nil
	}
	value := strings.TrimSpace(*raw)
	switch value {
	case "":
		return pgtype.Text{}, nil
	case "physical", "zoom":
		return pgtype.Text{String: value, Valid: true}, nil
	case "teacher_case", "none":
		return pgtype.Text{}, nil
	default:
		return pgtype.Text{}, fmt.Errorf("invalid sit-in method")
	}
}
