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

func TestServiceC_MVImport(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.MVImport(context.Background(), "world_a", "NORMAL")
	if err != nil {
		t.Fatalf("MVImport failed: %v", err)
	}
	if fx.lastReq.Command != "mv import world_a NORMAL" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_MVGameRule(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.MVGameRule(context.Background(), "keepInventory", "true", "lobby")
	if err != nil {
		t.Fatalf("MVGameRule failed: %v", err)
	}
	if fx.lastReq.Command != "mv gamerule keepInventory true lobby" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_MVUnload_RequireWorld(t *testing.T) {
	fx := &fakeExecutor{}
	svc := NewServiceC(fx)

	_, err := svc.MVUnload(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty world")
	}
}

func TestServiceC_MVSetAlias(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.MVSetAlias(context.Background(), "lobby", "mynamae")
	if err != nil {
		t.Fatalf("MVSetAlias failed: %v", err)
	}
	if fx.lastReq.Command != "mvm set alias mynamae lobby" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_LPGroupListMembers(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.LPGroupListMembers(context.Background(), "worldop")
	if err != nil {
		t.Fatalf("LPGroupListMembers failed: %v", err)
	}
	if fx.lastReq.Command != "lp group worldop listmembers" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_LPUserParentAdd(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.LPUserParentAdd(context.Background(), "vulcan9", "worldop", "")
	if err != nil {
		t.Fatalf("LPUserParentAdd failed: %v", err)
	}
	if fx.lastReq.Command != "lp user vulcan9 parent add worldop" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_LPUserParentRemove(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.LPUserParentRemove(context.Background(), "vulcan9", "worldop", "")
	if err != nil {
		t.Fatalf("LPUserParentRemove failed: %v", err)
	}
	if fx.lastReq.Command != "lp user vulcan9 parent remove worldop" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_LPUserParentAdd_WithWorldContext(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.LPUserParentAdd(context.Background(), "vulcan9", "worldmember", "i_10452")
	if err != nil {
		t.Fatalf("LPUserParentAdd failed: %v", err)
	}
	if fx.lastReq.Command != "lp user vulcan9 parent add worldmember world=i_10452" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}

func TestServiceC_LPUserParentRemove_WithWorldContext(t *testing.T) {
	fx := &fakeExecutor{resp: ParsedResponse{StatusCode: 200}}
	svc := NewServiceC(fx)

	_, err := svc.LPUserParentRemove(context.Background(), "vulcan9", "worldmember", "i_10452")
	if err != nil {
		t.Fatalf("LPUserParentRemove failed: %v", err)
	}
	if fx.lastReq.Command != "lp user vulcan9 parent remove worldmember world=i_10452" {
		t.Fatalf("unexpected command: %q", fx.lastReq.Command)
	}
}
