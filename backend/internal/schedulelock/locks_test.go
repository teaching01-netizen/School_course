package schedulelock

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNormalizeLockIDs_SortsDeduplicatesAndDropsNull(t *testing.T) {
	a := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	b := mustUUID(t, "00000000-0000-0000-0000-000000000002")

	got := normalizeLockIDs([]pgtype.UUID{b, {}, a, b})
	want := []pgtype.UUID{a, b}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestLockOrder_IsCanonicalAcrossAllResourceTypes(t *testing.T) {
	want := []resourceKind{courseResource, studentResource, teacherResource, roomResource, sessionResource, seriesResource}
	if !reflect.DeepEqual(lockOrder, want) {
		t.Fatalf("got=%v want=%v", lockOrder, want)
	}
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatal(err)
	}
	return id
}
