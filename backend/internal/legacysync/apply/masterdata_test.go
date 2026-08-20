package apply

import (
	"testing"

	"warwick-institute/internal/legacysync/normalize"
)

func TestMasterDataApplyRejectsMissingIdentity(t *testing.T) {
	service := &MasterDataService{}
	if _, err := service.ApplyTeacher(t.Context(), TeacherApplyRequest{Teacher: normalize.LegacyTeacher{Name: "Teacher"}}); err != ErrMissingMasterDataIdentity {
		t.Fatalf("error = %v, want ErrMissingMasterDataIdentity", err)
	}
	if _, err := service.ApplyRoom(t.Context(), RoomApplyRequest{Room: normalize.LegacyRoom{Name: "Room"}}); err != ErrMissingMasterDataIdentity {
		t.Fatalf("error = %v, want ErrMissingMasterDataIdentity", err)
	}
	if _, err := service.ApplySubject(t.Context(), SubjectApplyRequest{Subject: normalize.LegacySubject{Name: "Subject"}}); err != ErrMissingMasterDataIdentity {
		t.Fatalf("error = %v, want ErrMissingMasterDataIdentity", err)
	}
}

func TestDisabledPasswordHashIsDeterministicAndUnusable(t *testing.T) {
	first := disabledPasswordHash("78")
	if first != disabledPasswordHash("78") {
		t.Fatal("disabled teacher hash must be deterministic")
	}
	if len(first) < len("!legacy-disabled:")+16 || first[:len("!legacy-disabled:")] != "!legacy-disabled:" {
		t.Fatalf("hash = %q, want disabled marker", first)
	}
	if first == disabledPasswordHash("79") {
		t.Fatal("different legacy teachers must not share disabled hash")
	}
}
