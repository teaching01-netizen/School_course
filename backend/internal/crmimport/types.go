package crmimport

// UploadJobStatus tracks the state of a legacy upload job.
type UploadJobStatus string

const (
	UploadJobRunning   UploadJobStatus = "Running"
	UploadJobSucceeded UploadJobStatus = "Succeeded"
	UploadJobFailed    UploadJobStatus = "Failed"
)

// UploadResult is the summary returned after a successful upload.
type UploadResult struct {
	RowsParsed        int      `json:"rows_parsed"`
	RowsStored        int      `json:"rows_stored"`
	CyclesFound       []string `json:"cycles_found"`
	CoursesReconciled int      `json:"courses_reconciled"`
	StudentsAdded     int      `json:"students_added"`
	StudentsRemoved   int      `json:"students_removed"`
}
