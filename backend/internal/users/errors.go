package users

type ForbiddenError struct {
	Message string
}

func (e ForbiddenError) Error() string {
	if e.Message == "" {
		return "forbidden"
	}
	return e.Message
}

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Message == "" {
		return "invalid input"
	}
	return e.Message
}

const (
	RoleAdmin   = "Admin"
	RoleTeacher = "Teacher"
)
