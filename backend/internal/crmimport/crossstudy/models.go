package crossstudy

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

type AssignmentStatus string

const (
	StatusActive       AssignmentStatus = "active"
	StatusNotesChanged AssignmentStatus = "notes_changed"
	StatusOrphaned     AssignmentStatus = "orphaned"
	StatusPending      AssignmentStatus = "pending"
)

type Assignment struct {
	ID                              uuid.UUID
	SnapshotID                      uuid.UUID
	WCode                           string
	SourceCourseID                  uuid.UUID
	DestCourseAID                   uuid.UUID
	DestCourseBID                   uuid.UUID
	AssignedCourseID                uuid.UUID
	ExtraNoteSnapshot               string
	ExtraNoteHash                   string
	AssignedCourseEnrollmentCreated bool
	SourceCourseEnrollmentRemoved   bool
	SourceValid                     bool
	Status                          string
	CreatedAt                       time.Time
	UpdatedAt                       time.Time
}

type SaveAssignmentInput struct {
	WCode            string
	SourceCourseID   uuid.UUID
	SnapshotID       uuid.UUID
	DestCourseAID    uuid.UUID
	DestCourseBID    uuid.UUID
	AssignedCourseID uuid.UUID
	ExtraNoteText    string
}

type AssignmentSummary struct {
	ID                 string `json:"id"`
	WCode              string `json:"wcode"`
	FullName           string `json:"full_name"`
	SourceCourseName   string `json:"source_course_name"`
	SourceCourseID     string `json:"source_course_id"`
	AssignedCourseName string `json:"assigned_course_name"`
	AssignedCourseID   string `json:"assigned_course_id"`
	Status             string `json:"status"`
	UpdatedAt          string `json:"updated_at"`
}

type StudentLookupResponse struct {
	Student           *StudentInfo   `json:"student"`
	CRMRow            *CRMRowInfo    `json:"crm_row"`
	CurrentAssignment *AssignmentDTO `json:"current_assignment,omitempty"`
}

type StudentInfo struct {
	ID       string `json:"id"`
	WCode    string `json:"wcode"`
	FullName string `json:"full_name"`
}

type CRMRowInfo struct {
	SnapshotID string `json:"snapshot_id"`
	CourseName string `json:"course_name"`
	CourseID   string `json:"course_id"`
	ExtraNote  string `json:"extra_note"`
	ImportedAt string `json:"imported_at"`
}

type CourseRef struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	SubjectName string `json:"subject_name"`
}

type AssignmentDTO struct {
	ID                string     `json:"id"`
	DestCourseA       *CourseRef `json:"dest_course_a"`
	DestCourseB       *CourseRef `json:"dest_course_b"`
	AssignedCourseID  string     `json:"assigned_course_id"`
	Status            string     `json:"status"`
	ExtraNoteSnapshot string     `json:"extra_note_snapshot"`
	SourceValid       bool       `json:"source_valid"`
	UpdatedAt         string     `json:"updated_at"`
}

type AssignmentChange struct {
	ID                uuid.UUID
	WCode             string
	CurrentNote       string
	CurrentCourseName string
	StoredHash        string
}

func hashExtraNote(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
