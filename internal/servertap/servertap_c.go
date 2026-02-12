package servertap

import (
	"context"
	"fmt"
	"strings"
)

type Executor interface {
	Execute(ctx context.Context, req ExecuteRequest) (ParsedResponse, error)
}

type ServiceC struct {
	executor Executor
}

func NewServiceC(executor Executor) *ServiceC {
	return &ServiceC{executor: executor}
}

func (s *ServiceC) OPUser(ctx context.Context, user string) (ParsedResponse, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return ParsedResponse{}, fmt.Errorf("user is required")
	}
	cmd := NewCommandBuilder("op").Arg(user).Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) DEOPUser(ctx context.Context, user string) (ParsedResponse, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return ParsedResponse{}, fmt.Errorf("user is required")
	}
	cmd := NewCommandBuilder("deop").Arg(user).Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) ListOps(ctx context.Context) (ParsedResponse, error) {
	cmd := NewCommandBuilder("ops").Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

/*
Legacy command wrappers are intentionally disabled for now:
- mv import/unload/load/remove/delete/gamerule/alias
- luckperms parent add/remove/group

If needed later, restore from git history and move behind feature flags.

func (s *ServiceC) MVImport(ctx context.Context, name string, env string) (ParsedResponse, error) { ... }
func (s *ServiceC) MVUnload(ctx context.Context, world string) (ParsedResponse, error) { ... }
func (s *ServiceC) MVLoad(ctx context.Context, world string) (ParsedResponse, error) { ... }
func (s *ServiceC) MVRemove(ctx context.Context, world string) (ParsedResponse, error) { ... }
func (s *ServiceC) MVDelete(ctx context.Context, world string) (ParsedResponse, error) { ... }
func (s *ServiceC) MVGameRule(ctx context.Context, rule string, value string, world string) (ParsedResponse, error) { ... }
func (s *ServiceC) MVSetAlias(ctx context.Context, world string, alias string) (ParsedResponse, error) { ... }
func (s *ServiceC) LPGroupListMembers(ctx context.Context, group string) (ParsedResponse, error) { ... }
func (s *ServiceC) LPUserParentAdd(ctx context.Context, user string, group string, world string) (ParsedResponse, error) { ... }
func (s *ServiceC) LPUserParentRemove(ctx context.Context, user string, group string, world string) (ParsedResponse, error) { ... }
*/
