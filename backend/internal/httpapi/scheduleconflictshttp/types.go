package scheduleconflictshttp

type sessionDTO struct {
	SessionID   string  `json:"session_id"`
	CourseID    string  `json:"course_id"`
	CourseCode  string  `json:"course_code"`
	CourseName  string  `json:"course_name"`
	SubjectID   string  `json:"subject_id"`
	SubjectName string  `json:"subject_name"`
	TeacherID   string  `json:"teacher_id"`
	TeacherName string  `json:"teacher_name"`
	RoomID      *string `json:"room_id"`
	RoomName    *string `json:"room_name"`
	StartAt     string  `json:"start_at"`
	EndAt       string  `json:"end_at"`
}

type studentDTO struct {
	StudentID string `json:"student_id"`
	WCode     string `json:"wcode"`
	FullName  string `json:"full_name"`
}

type resourceDTO struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type conflictDTO struct {
	ID                  string       `json:"id"`
	ConflictType        string       `json:"conflict_type"`
	PrimarySession      sessionDTO   `json:"primary_session"`
	ConflictingSessions []sessionDTO `json:"conflicting_sessions"`
	AffectedStudents    []studentDTO `json:"affected_students"`
	SharedResource      resourceDTO  `json:"shared_resource"`
	DetectedAt          string       `json:"detected_at"`
}

type summaryDTO struct {
	TotalConflicts  int `json:"total_conflicts"`
	RoomOverlaps    int `json:"room_overlaps"`
	TeacherOverlaps int `json:"teacher_overlaps"`
	StudentOverlaps int `json:"student_overlaps"`
}

type listResponse struct {
	Items      []conflictDTO `json:"items"`
	TotalCount int           `json:"total_count"`
	Offset     int           `json:"offset"`
	Limit      int           `json:"limit"`
	Summary    summaryDTO    `json:"summary"`
}
