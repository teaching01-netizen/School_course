package apply

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"warwick-institute/internal/legacysync/normalize"
)

func TestValidateCourseAggregateRequiresStableCourseIdentity(t *testing.T) {
	request := CourseApplyRequest{Aggregate: normalize.LegacyCourseAggregate{Course: normalize.LegacyCourse{LegacyID: "7306"}}}
	if !errors.Is(ValidateCourseAggregate(request), ErrMissingCourseIdentity) {
		t.Fatal("expected missing course identity error")
	}
}

func TestValidateCourseAggregateRejectsInvalidSourceVocabulary(t *testing.T) {
	request := CourseApplyRequest{
		CourseID:       pgtype.UUID{Valid: true},
		LegacyCourseID: "7306",
		Aggregate: normalize.LegacyCourseAggregate{
			Course: normalize.LegacyCourse{LegacyID: "7306", Status: "deleted"},
		},
	}
	if err := ValidateCourseAggregate(request); err == nil {
		t.Fatal("expected invalid status error")
	}
}
