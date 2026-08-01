// Package teacherpolicy centralizes the rule for which users may be assigned
// to teach. Scheduling (session/series creation) and course administration
// must both consult this single policy instead of duplicating role checks.
package teacherpolicy

// RoleTeacher is the only role that may teach. The users table constrains
// role to ('Admin','Teacher') via CHECK and soft-deletes via deleted_at;
// there is no separate "active" column.
const RoleTeacher = "Teacher"

// CanTeach reports whether a user with the given role is eligible to teach.
func CanTeach(role string) bool {
	return role == RoleTeacher
}
