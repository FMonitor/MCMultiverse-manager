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

func (s *ServiceC) MVImport(ctx context.Context, name string, env string) (ParsedResponse, error) {
	name = strings.TrimSpace(name)
	env = strings.TrimSpace(env)
	if name == "" || env == "" {
		return ParsedResponse{}, fmt.Errorf("name and env are required")
	}
	cmd := NewCommandBuilder("mv").RawArg("import").Arg(name).Arg(env).Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) MVUnload(ctx context.Context, world string) (ParsedResponse, error) {
	world = strings.TrimSpace(world)
	if world == "" {
		return ParsedResponse{}, fmt.Errorf("world is required")
	}
	cmd := NewCommandBuilder("mv").RawArg("unload").Arg(world).Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) MVLoad(ctx context.Context, world string) (ParsedResponse, error) {
	world = strings.TrimSpace(world)
	if world == "" {
		return ParsedResponse{}, fmt.Errorf("world is required")
	}
	cmd := NewCommandBuilder("mv").RawArg("load").Arg(world).Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) MVRemove(ctx context.Context, world string) (ParsedResponse, error) {
	world = strings.TrimSpace(world)
	if world == "" {
		return ParsedResponse{}, fmt.Errorf("world is required")
	}
	cmd := NewCommandBuilder("mv").RawArg("remove").Arg(world).Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) MVDelete(ctx context.Context, world string) (ParsedResponse, error) {
	world = strings.TrimSpace(world)
	if world == "" {
		return ParsedResponse{}, fmt.Errorf("world is required")
	}
	cmd := NewCommandBuilder("mv").RawArg("delete").Arg(world).Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) MVGameRule(ctx context.Context, rule string, value string, world string) (ParsedResponse, error) {
	rule = strings.TrimSpace(rule)
	value = strings.TrimSpace(value)
	world = strings.TrimSpace(world)
	if rule == "" || value == "" {
		return ParsedResponse{}, fmt.Errorf("rule and value are required")
	}

	b := NewCommandBuilder("mv").RawArg("gamerule").Arg(rule).Arg(value)
	if world != "" {
		b.Arg(world)
	}
	return s.executor.Execute(ctx, ExecuteRequest{Command: b.Build()})
}

func (s *ServiceC) MVSetAlias(ctx context.Context, world string, alias string) (ParsedResponse, error) {
	world = strings.TrimSpace(world)
	alias = strings.TrimSpace(alias)
	if world == "" || alias == "" {
		return ParsedResponse{}, fmt.Errorf("world and alias are required")
	}
	cmd := NewCommandBuilder("mvm").RawArg("set").RawArg("alias").Arg(alias).Arg(world).Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) LPGroupListMembers(ctx context.Context, group string) (ParsedResponse, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return ParsedResponse{}, fmt.Errorf("group is required")
	}
	cmd := NewCommandBuilder("lp").RawArg("group").Arg(group).RawArg("listmembers").Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) LPUserParentAdd(ctx context.Context, user string, group string, world string) (ParsedResponse, error) {
	user = strings.TrimSpace(user)
	group = strings.TrimSpace(group)
	world = strings.TrimSpace(world)
	if user == "" || group == "" {
		return ParsedResponse{}, fmt.Errorf("user and group are required")
	}
	b := NewCommandBuilder("lp").RawArg("user").Arg(user).RawArg("parent").RawArg("add").Arg(group)
	if world != "" {
		b.Arg("world=" + world)
	}
	cmd := b.Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}

func (s *ServiceC) LPUserParentRemove(ctx context.Context, user string, group string, world string) (ParsedResponse, error) {
	user = strings.TrimSpace(user)
	group = strings.TrimSpace(group)
	world = strings.TrimSpace(world)
	if user == "" || group == "" {
		return ParsedResponse{}, fmt.Errorf("user and group are required")
	}
	b := NewCommandBuilder("lp").RawArg("user").Arg(user).RawArg("parent").RawArg("remove").Arg(group)
	if world != "" {
		b.Arg("world=" + world)
	}
	cmd := b.Build()
	return s.executor.Execute(ctx, ExecuteRequest{Command: cmd})
}
