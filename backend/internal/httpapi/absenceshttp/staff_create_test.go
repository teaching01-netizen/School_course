package absenceshttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDispatchStaffCreate_RouteRegistered(t *testing.T) {
	server := &server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/staff-create", nil)
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatal("POST /absences/staff-create should route to a handler, got 404")
	}
}

func TestDispatchStaffCreate_GetMethodReturns404(t *testing.T) {
	server := &server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/absences/staff-create", nil)
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /absences/staff-create should return 404, got %d", w.Code)
	}
}

func TestDispatchStaffCreate_PutMethodReturns404(t *testing.T) {
	server := &server{}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/absences/staff-create", nil)
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("PUT /absences/staff-create should return 404, got %d", w.Code)
	}
}

func TestDispatchSendSuccessSMS_RouteRegistered(t *testing.T) {
	server := &server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/00000000-0000-0000-0000-000000000001/send-success-sms", nil)
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatal("POST /absences/{id}/send-success-sms should route to a handler, got 404")
	}
}

func TestDispatchSendSuccessSMS_GetMethodReturns404(t *testing.T) {
	server := &server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/absences/00000000-0000-0000-0000-000000000001/send-success-sms", nil)
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /absences/{id}/send-success-sms should return 404, got %d", w.Code)
	}
}

func TestDispatchBatchSendSuccessSMS_RouteRegistered(t *testing.T) {
	server := &server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/batch-send-success-sms", nil)
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatal("POST /absences/batch-send-success-sms should route to a handler, got 404")
	}
}

func TestDispatchBatchSendSuccessSMS_GetMethodReturns404(t *testing.T) {
	server := &server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/absences/batch-send-success-sms", nil)
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /absences/batch-send-success-sms should return 404, got %d", w.Code)
	}
}

func TestDispatchBatchSendSuccessSMS_PutMethodReturns404(t *testing.T) {
	server := &server{}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/absences/batch-send-success-sms", nil)
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("PUT /absences/batch-send-success-sms should return 404, got %d", w.Code)
	}
}

func TestBatchSendSuccessSMS_AcceptsValidJSONBody(t *testing.T) {
	server := &server{}
	body := bytes.NewBufferString(`{"ids":["00000000-0000-0000-0000-000000000001","00000000-0000-0000-0000-000000000002"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/absences/batch-send-success-sms", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleAbsencesDispatch(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatal("POST /absences/batch-send-success-sms should route to handler, got 404")
	}
}
