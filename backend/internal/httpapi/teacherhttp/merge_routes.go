package teacherhttp

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"warwick-institute/internal/teachermerge"
)

type mergeAccountDTO struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Deleted   bool   `json:"deleted"`
	CreatedAt string `json:"created_at"`
	IsLegacy  bool   `json:"is_legacy"`
}

type mergeImpactDTO struct {
	SessionsLive        int64 `json:"sessions_live"`
	SessionsDeleted     int64 `json:"sessions_deleted"`
	Courses             int64 `json:"courses"`
	Series              int64 `json:"series"`
	CourseTeacherRows   int64 `json:"course_teacher_rows"`
	AvailabilityBlocks  int64 `json:"availability_blocks"`
	ExternalRefMappings int64 `json:"external_ref_mappings"`
	ConflictSessions    int64 `json:"conflict_sessions"`
}

type mergePreviewDTO struct {
	Duplicate mergeAccountDTO `json:"duplicate"`
	Canonical mergeAccountDTO `json:"canonical"`
	Impact    mergeImpactDTO  `json:"impact"`
}

type mergeResultDTO struct {
	Impact    mergeImpactDTO  `json:"impact"`
	Canonical mergeAccountDTO `json:"canonical"`
}

// mergeService returns nil when the pool is absent (test wiring); handlers
// translate that to a 500 rather than panicking on a nil receiver.
func (s *server) mergeService() *teachermerge.Service {
	if s.deps.DB == nil {
		return nil
	}
	return teachermerge.New(s.deps.DB, s.deps.Q)
}

func mergeAccountToDTO(a teachermerge.Account) mergeAccountDTO {
	return mergeAccountDTO{
		ID:        a.ID.String(),
		Username:  a.Username,
		FullName:  a.FullName,
		Email:     a.Email,
		Role:      a.Role,
		Deleted:   a.Deleted,
		CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339),
		IsLegacy:  a.IsLegacy,
	}
}

func (s *server) writeMergeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, teachermerge.ErrSameAccount):
		s.a.WriteErr(w, http.StatusBadRequest, "same_account", "Duplicate and canonical must be different accounts")
	case errors.Is(err, teachermerge.ErrAccountNotFound):
		s.a.WriteErr(w, http.StatusNotFound, "account_not_found", "Teacher account not found")
	case errors.Is(err, teachermerge.ErrNotTeacher):
		s.a.WriteErr(w, http.StatusBadRequest, "not_teacher", "Both accounts must be teachers")
	case errors.Is(err, teachermerge.ErrCanonicalInactive):
		s.a.WriteErr(w, http.StatusConflict, "canonical_inactive", "The account to keep is deactivated")
	case errors.Is(err, teachermerge.ErrCanonicalLegacy):
		s.a.WriteErr(w, http.StatusConflict, "canonical_legacy", "Pick the account the teacher logs in with, not another legacy duplicate")
	case errors.Is(err, teachermerge.ErrAlreadyMerged):
		s.a.WriteErr(w, http.StatusConflict, "already_merged", "This duplicate is already deactivated")
	default:
		status, code, msg := s.a.ClassifyDBErr(err)
		s.a.WriteErr(w, status, code, msg)
	}
}

func (s *server) parseMergeIDs(w http.ResponseWriter, duplicateRaw, canonicalRaw string) (uuid.UUID, uuid.UUID, bool) {
	duplicate, err := s.a.ParseUUID(duplicateRaw)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_duplicate_user_id", "Invalid duplicate_user_id")
		return uuid.Nil, uuid.Nil, false
	}
	canonical, err := s.a.ParseUUID(canonicalRaw)
	if err != nil {
		s.a.WriteErr(w, http.StatusBadRequest, "bad_canonical_user_id", "Invalid canonical_user_id")
		return uuid.Nil, uuid.Nil, false
	}
	return uuid.UUID(duplicate.Bytes), uuid.UUID(canonical.Bytes), true
}

func (s *server) handleMergePreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.a.MustAdmin(w, r); !ok {
		return
	}
	duplicateID, canonicalID, ok := s.parseMergeIDs(w, r.URL.Query().Get("duplicate_user_id"), r.URL.Query().Get("canonical_user_id"))
	if !ok {
		return
	}
	service := s.mergeService()
	if service == nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	preview, err := service.Preview(r.Context(), duplicateID, canonicalID)
	if err != nil {
		s.writeMergeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	s.a.WriteJSON(w, http.StatusOK, mergePreviewDTO{
		Duplicate: mergeAccountToDTO(preview.Duplicate),
		Canonical: mergeAccountToDTO(preview.Canonical),
		Impact:    mergeImpactDTO(preview.Impact),
	})
}

func (s *server) handleMerge(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.a.MustAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		DuplicateUserID string `json:"duplicate_user_id"`
		CanonicalUserID string `json:"canonical_user_id"`
	}
	if err := s.a.DecodeJSON(w, r, &body); err != nil {
		return
	}
	duplicateID, canonicalID, ok := s.parseMergeIDs(w, body.DuplicateUserID, body.CanonicalUserID)
	if !ok {
		return
	}
	service := s.mergeService()
	if service == nil {
		s.a.WriteErr(w, http.StatusInternalServerError, "internal", "Internal error")
		return
	}
	result, err := service.Merge(r.Context(), admin.ID, duplicateID, canonicalID)
	if err != nil {
		s.writeMergeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	s.a.WriteJSON(w, http.StatusOK, mergeResultDTO{
		Impact:    mergeImpactDTO(result.Impact),
		Canonical: mergeAccountToDTO(result.Canonical),
	})
}
