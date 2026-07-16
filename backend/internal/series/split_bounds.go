package series

type SplitPartition struct {
	Retained  int
	Remaining int
	InPlace   bool
}

func partitionCountBoundedSplit(retained, targetTotal int) (SplitPartition, error) {
	if retained < 0 || targetTotal <= retained {
		return SplitPartition{}, newValidationError(
			"invalid_recurrence_count",
			"total count must exceed retained occurrences",
		)
	}
	return SplitPartition{
		Retained:  retained,
		Remaining: targetTotal - retained,
		InPlace:   retained == 0,
	}, nil
}
