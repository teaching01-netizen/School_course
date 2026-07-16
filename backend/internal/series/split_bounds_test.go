package series

import "testing"

func TestPartitionCountBoundedSplit_PreservesTargetTotal(t *testing.T) {
	got, err := partitionCountBoundedSplit(4, 10)
	if err != nil || got.Retained != 4 || got.Remaining != 6 || got.InPlace {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestPartitionCountBoundedSplit_FirstOccurrenceUsesInPlaceEdit(t *testing.T) {
	got, err := partitionCountBoundedSplit(0, 10)
	if err != nil || !got.InPlace || got.Remaining != 10 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestPartitionCountBoundedSplit_RejectsTargetBelowRetainedPrefix(t *testing.T) {
	if _, err := partitionCountBoundedSplit(6, 6); err == nil {
		t.Fatal("expected invalid total")
	}
}
