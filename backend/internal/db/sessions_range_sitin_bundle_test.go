package db

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func testBundleUUID(value byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: value}, Valid: true}
}

func TestBundleSessionCourseIDsForDiscoverySeparatesSATHistory(t *testing.T) {
	ordinaryID := testBundleUUID(1)
	directMappedID := testBundleUUID(2)
	mergeGroupID := testBundleUUID(3)
	mergeMemberInScopeID := testBundleUUID(4)
	mergeMemberOutOfScopeID := testBundleUUID(5)

	out := &SitInBundleV2{
		ScopeCourses:     []SubjectCourseV2{{ID: ordinaryID}},
		SatMemberCourses: []SubjectCourseV2{{ID: mergeMemberInScopeID}},
		SatMappings: []SatVerbalPolicyCourseMapping{
			{CourseID: directMappedID},
			{MergeGroupID: mergeGroupID},
		},
		MergeMembers: map[string][]pgtype.UUID{
			uuidBytesString(mergeGroupID): {mergeMemberInScopeID, mergeMemberOutOfScopeID},
		},
	}

	all, fullHistory, bounded := bundleSessionCourseIDsForDiscovery(out)
	if len(all) != 4 {
		t.Fatalf("all session courses = %d, want 4", len(all))
	}
	if len(fullHistory) != 3 {
		t.Fatalf("full-history courses = %d, want 3", len(fullHistory))
	}
	if len(bounded) != 1 || bounded[0] != ordinaryID {
		t.Fatalf("bounded courses = %#v, want only ordinary course %v", bounded, ordinaryID)
	}

	seen := make(map[string]struct{}, len(all))
	for _, id := range all {
		key := uuidBytesString(id)
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate session course %v", id)
		}
		seen[key] = struct{}{}
	}
}

func TestBundleVisibleCourseIDsIncludesDirectSATMappings(t *testing.T) {
	scopeID := testBundleUUID(11)
	satMemberID := testBundleUUID(12)
	directMappedID := testBundleUUID(13)
	duplicateID := testBundleUUID(14)

	got := bundleVisibleCourseIDs(
		[]SubjectCourseV2{{ID: scopeID}, {ID: duplicateID}},
		[]SubjectCourseV2{{ID: satMemberID}, {ID: duplicateID}},
		[]SatVerbalPolicyCourseMapping{
			{CourseID: directMappedID, Active: true},
			{CourseID: duplicateID, Active: true},
			{CourseID: pgtype.UUID{}, Active: true}, // merge-group mappings have no direct course ID
		},
	)

	want := []string{
		uuidBytesString(scopeID),
		uuidBytesString(duplicateID),
		uuidBytesString(satMemberID),
		uuidBytesString(directMappedID),
	}
	if len(got) != len(want) {
		t.Fatalf("visibility IDs = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("visibility IDs = %#v, want %#v", got, want)
		}
	}
}
