package teachermerge

import "errors"

var (
	ErrSameAccount       = errors.New("duplicate and canonical must be different accounts")
	ErrAccountNotFound   = errors.New("teacher account not found")
	ErrNotTeacher        = errors.New("account is not a teacher")
	ErrCanonicalInactive = errors.New("canonical account is deactivated")
	ErrCanonicalLegacy   = errors.New("canonical account is a legacy duplicate; pick the account the teacher logs in with")
	ErrAlreadyMerged     = errors.New("duplicate account is already deactivated")
)
