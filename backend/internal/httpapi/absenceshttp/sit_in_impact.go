package absenceshttp

import (
	sqldb "warwick-institute/internal/db"
	"warwick-institute/internal/snapshot"
)

type sitInImpactDTO struct {
	SessionID        string                      `json:"session_id"`
	OriginalSnapshot *snapshot.SessionSnapshotV1 `json:"original_snapshot"`
	SnapshotQuality  string                      `json:"snapshot_quality"`
	CurrentSession   *absenceSessionDTO          `json:"current_session"`
}

func (s *server) sitInImpactDTO(row sqldb.AbsenceSitInImpact) sitInImpactDTO {
	id, _ := s.a.UUIDString(row.SessionID)
	dto := sitInImpactDTO{SessionID: id, SnapshotQuality: "unavailable"}
	for _, evidence := range []struct {
		data    []byte
		quality string
	}{
		{row.AssignmentSnapshot, row.AssignmentQuality}, {row.IssueSnapshot, row.IssueQuality},
	} {
		if evidence.quality != "exact" && evidence.quality != "reconstructed" {
			continue
		}
		original, err := snapshot.DecodeSessionSnapshotV1(evidence.data)
		if err == nil && original.SessionID.String() == id {
			dto.OriginalSnapshot = &original
			dto.SnapshotQuality = evidence.quality
			break
		}
	}
	if row.Current.SessionID.Valid {
		current := s.sessionDTO([]sqldb.ManagedAbsenceSession{row.Current})[0]
		dto.CurrentSession = &current
	}
	return dto
}
