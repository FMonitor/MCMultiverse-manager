package servertap

import (
	"context"
	"testing"
)

type fakeExecutor struct {
	lastReq ExecuteRequest
	resp    ParsedResponse
	err     error
}

func (f *fakeExecutor) Execute(_ context.Context, req ExecuteRequest) (ParsedResponse, error) {
	f.lastReq = req
	return f.resp, f.err
}

func TestServiceC_OPUser(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.OPUser(context.Background(), "vulcan9")
	if err != nil {
		t.Fatalf("OPUser failed: %v", err)
	}
	if fx.lastReq.Command != "op vulcan9" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_DEOPUser(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.DEOPUser(context.Background(), "vulcan9")
	if err != nil {
		t.Fatalf("DEOPUser failed: %v", err)
	}
	if fx.lastReq.Command != "deop vulcan9" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_ListOps(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.ListOps(context.Background())
	if err != nil {
		t.Fatalf("ListOps failed: %v", err)
	}
	if fx.lastReq.Command != "ops" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_OPUser_RequireUser(t *testing.T) {
	fx := &fakeExecutor{}
	svc := NewServiceC(fx)

	_, err := svc.OPUser(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty user")
	}
}

func TestServiceC_DEOPUser_RequireUser(t *testing.T) {
	fx := &fakeExecutor{}
	svc := NewServiceC(fx)

	_, err := svc.DEOPUser(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty user")
	}
}
