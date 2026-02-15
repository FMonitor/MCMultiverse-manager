package cmdreceiver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type serviceMock struct {
	status int
	resp   WorldCommandResponse
	called bool
}

func (m *serviceMock) HandleWorldCommand(ctx context.Context, req WorldCommandRequest) (int, WorldCommandResponse) {
	m.called = true
	if m.status == 0 {
		m.status = http.StatusOK
	}
	if m.resp.Status == "" {
		m.resp.Status = "accepted"
	}
	return m.status, m.resp
}

func (m *serviceMock) HandlePlayerJoin(ctx context.Context, actorUUID string, actorName string) (int, WorldCommandResponse) {
	m.called = true
	if m.status == 0 {
		m.status = http.StatusOK
	}
	if m.resp.Status == "" {
		m.resp.Status = "accepted"
	}
	return m.status, m.resp
}

func TestHandleWorldCommand_MethodNotAllowed(t *testing.T) {
	h := NewHandlerI(&serviceMock{})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/v1/cmd/world", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got status=%d", rec.Code)
	}
}

func TestHandleWorldCommand_PostSuccess(t *testing.T) {
	sm := &serviceMock{status: http.StatusOK, resp: WorldCommandResponse{Status: "accepted", Message: "ok"}}
	h := NewHandlerI(sm)
	mux := http.NewServeMux()
	h.Register(mux)

	form := url.Values{}
	form.Set("action", "create")
	form.Set("actor_uuid", "11111111-1111-1111-1111-111111111111")
	req := httptest.NewRequest(http.MethodPost, "/v1/cmd/world", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !sm.called {
		t.Fatalf("service should be called")
	}
}
