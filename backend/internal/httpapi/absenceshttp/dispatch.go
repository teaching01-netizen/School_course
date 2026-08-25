package absenceshttp

import (
	"net/http"
	"strings"
)

func (s *server) handleAbsencesDispatch(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/v1/absences"

	path := strings.TrimPrefix(r.URL.Path, prefix)
	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodGet:
			s.handleAbsenceInbox(w, r)
		case http.MethodPost:
			s.handleAbsenceCreate(w, r)
		default:
			s.writeMethodNotAllowed(w)
		}
		return
	}

	if !strings.HasPrefix(path, "/") {
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Not found")
		return
	}

	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	switch parts[0] {

	case "sit-in-options":
		if r.Method == http.MethodGet {
			s.handleSitInOptions(w, r)
			return
		}
	case "sessions-in-range":
		if r.Method == http.MethodGet {
			s.handleSessionsInRange(w, r)
			return
		}
	case "parent-verification":
		switch {
		case len(parts) == 2 && parts[1] == "send" && r.Method == http.MethodPost:
			s.handleParentVerificationSend(w, r)
			return
		case len(parts) == 2 && parts[1] == "verify" && r.Method == http.MethodPost:
			s.handleParentVerificationVerify(w, r)
			return
		case len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodPost:
			s.handleParentVerificationStatus(w, r)
			return
		}
	case "stats":
		if r.Method == http.MethodGet {
			s.handleAbsenceStats(w, r)
			return
		}
	case "dashboard":
		if r.Method == http.MethodGet {
			s.handleAbsenceDashboard(w, r)
			return
		}
	case "export":
		if r.Method == http.MethodGet {
			s.handleAbsenceExport(w, r)
			return
		}
	case "staff-create":
		if r.Method == http.MethodPost {
			if r.Header.Get("X-Staff-Batch") == "true" {
				s.handleStaffCreateAbsenceBatch(w, r)
				return
			}
			s.handleStaffCreateAbsence(w, r)
			return
		}
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Not found")
		return
	case "staff-create-batch":
		if r.Method == http.MethodPost {
			s.handleStaffCreateAbsenceBatch(w, r)
			return
		}
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Not found")
		return
	case "batch-send-success-sms":
		if r.Method == http.MethodPost {
			s.handleBatchSendSuccessSMS(w, r)
			return
		}
		s.a.WriteErr(w, http.StatusNotFound, "not_found", "Not found")
		return
	case "batch-status":
		if r.Method == http.MethodPost {
			s.handleBatchStatus(w, r)
			return
		}
	case "batch":
		if r.Method == http.MethodPost {
			s.handleAbsenceBatchCreate(w, r)
			return
		}
	case "calendar":
		if r.Method == http.MethodGet {
			s.handleCalendar(w, r)
			return
		}
	default:
		r.SetPathValue("id", parts[0])
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			s.handleAbsenceGet(w, r)
			return
		case len(parts) == 1 && r.Method == http.MethodPost:
			s.writeMethodNotAllowed(w)
			return
		case len(parts) == 2 && r.Method == http.MethodGet && parts[1] == "timeline":
			s.handleAbsenceTimeline(w, r)
			return
		case len(parts) == 2 && r.Method == http.MethodGet && parts[1] == "sit-in-candidates":
			s.handleSitInCandidates(w, r)
			return
		case len(parts) == 2 && r.Method == http.MethodPut && parts[1] == "status":
			s.handleAbsenceStatusUpdate(w, r)
			return
		case len(parts) == 2 && r.Method == http.MethodPut && parts[1] == "notes":
			s.handleAbsenceNotesUpdate(w, r)
			return
		case len(parts) == 2 && r.Method == http.MethodPut && parts[1] == "sit-in":
			s.handleSitInOverride(w, r)
			return
		case len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "cancel":
			s.handlePendingCancel(w, r)
			return
		case len(parts) == 2 && r.Method == http.MethodPost && parts[1] == "send-success-sms":
			s.handleSendSuccessSMS(w, r)
			return
		case len(parts) == 1 && r.Method == http.MethodDelete:
			s.handleAbsenceDelete(w, r)
			return
		}
	}

	s.a.WriteErr(w, http.StatusNotFound, "not_found", "Not found")
}

func (s *server) writeMethodNotAllowed(w http.ResponseWriter) {
	s.a.WriteErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
}
