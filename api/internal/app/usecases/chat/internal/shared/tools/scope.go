package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

var (
	ErrUnauthorized = problem.New(problem.Forbidden, "agenttools: unauthorized")
	ErrOutOfScope   = errors.New("agenttools: out of scope")
	ErrOwnThread    = errors.New(
		"agenttools: that chat is a thread of this one; a chat reads the chats ABOVE it, never below",
	)
	ErrToolsDisabled = errors.New(
		"agenttools: Crowbar's tools are switched off for this provider " +
			"(Settings → Providers → Tools); no tool call will be served",
	)
)

type RunnerReader interface {
	RunnerByID(runnerID string) (agents.Runner, bool)
	RelayRunner(ctx context.Context, relayID string) (agents.Runner, bool, error)
}

type ChatReader interface {
	Get(ctx context.Context, chatID string) (domain.Chat, error)
	ListChats(ctx context.Context) ([]domain.Chat, error)
}

type WorkspaceLister interface {
	Get(ctx context.Context, wsID string) (domain.Workspace, error)
	List(ctx context.Context) ([]domain.Workspace, error)
}

type Caller struct {
	RunnerID   string
	ChatID     string
	ProviderID string
	Workspace  domain.Workspace
	visible    *visibleSet
}

func (c Caller) Visible() ([]domain.Workspace, error) {
	if c.visible == nil {
		return nil, fmt.Errorf("agenttools: visible: caller carries no visibility set: %w", ErrUnauthorized)
	}
	return c.visible.get()
}

func (c Caller) CanSee(wsID string) bool {
	all, err := c.Visible()
	if err != nil {
		return false
	}
	for _, w := range all {
		if w.ID == wsID {
			return true
		}
	}
	return false
}

type visibleSet struct {
	mu     sync.Mutex
	load   func() ([]domain.Workspace, error)
	loaded bool
	all    []domain.Workspace
	err    error
}

func (v *visibleSet) get() ([]domain.Workspace, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.loaded {
		return v.all, v.err
	}
	v.loaded = true
	v.all, v.err = v.load()
	return v.all, v.err
}

type Resolver struct {
	minter     *TokenMinter
	runners    RunnerReader
	workspaces WorkspaceLister
}

func NewResolver(
	minter *TokenMinter,
	runners RunnerReader,
	workspaces WorkspaceLister,
) *Resolver {
	return &Resolver{minter: minter, runners: runners, workspaces: workspaces}
}

func (r *Resolver) Resolve(ctx context.Context, runnerID, token string) (Caller, error) {
	if r.minter == nil || r.runners == nil || r.workspaces == nil {
		return Caller{}, fmt.Errorf("agenttools: resolve: resolver not configured: %w", ErrUnauthorized)
	}
	if runnerID == "" || !r.minter.Verify(runnerID, token) {
		return Caller{}, ErrUnauthorized
	}
	runner, ok := r.runners.RunnerByID(runnerID)
	if !ok {
		return Caller{}, fmt.Errorf("agenttools: resolve runner: %w", ErrUnauthorized)
	}
	return r.caller(ctx, runner)
}

func (r *Resolver) ResolveRelay(ctx context.Context, relayID, token string) (Caller, error) {
	if r.minter == nil || r.runners == nil || r.workspaces == nil {
		return Caller{}, fmt.Errorf("agenttools: resolve relay: resolver not configured: %w", ErrUnauthorized)
	}
	if !r.minter.VerifyRelay(relayID, token) {
		return Caller{}, ErrUnauthorized
	}
	runner, ok, err := r.runners.RelayRunner(ctx, relayID)
	if err != nil {
		return Caller{}, fmt.Errorf("agenttools: resolve relay: %w", errors.Join(ErrUnauthorized, err))
	}
	if !ok {
		return Caller{}, fmt.Errorf("agenttools: resolve relay: no live runner: %w", ErrUnauthorized)
	}
	return r.caller(ctx, runner)
}

func (r *Resolver) caller(ctx context.Context, runner agents.Runner) (Caller, error) {
	ws, err := r.workspaces.Get(ctx, runner.WorkspaceID)
	if err != nil {
		return Caller{}, fmt.Errorf("agenttools: get caller workspace: %w", errors.Join(ErrUnauthorized, err))
	}
	return Caller{
		RunnerID:   runner.ID,
		ChatID:     runner.CurrentChatID,
		ProviderID: runner.ProviderID,
		Workspace:  ws,
		visible:    &visibleSet{load: r.visibleLoader(ctx, ws)},
	}, nil
}

func (r *Resolver) visibleLoader(
	ctx context.Context,
	ws domain.Workspace,
) func() ([]domain.Workspace, error) {
	return func() ([]domain.Workspace, error) {
		all, err := r.workspaces.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("agenttools: list workspaces: %w", err)
		}
		return visibleFrom(ws, all), nil
	}
}

func visibleFrom(caller domain.Workspace, all []domain.Workspace) []domain.Workspace {
	live := filter(all, func(w domain.Workspace) bool {
		return w.ID == caller.ID || w.Status != domain.WorkspaceStatusDeleted
	})
	switch {
	case caller.Kind == domain.WorkspaceKindHome:
		return filter(live, func(w domain.Workspace) bool { return w.ProjectID == caller.ProjectID })
	case caller.IsDefault:
		return filter(live, func(w domain.Workspace) bool {
			return w.ProjectID == caller.ProjectID && w.RepoID == caller.RepoID
		})
	default:
		return descendants(caller, live)
	}
}

func descendants(caller domain.Workspace, all []domain.Workspace) []domain.Workspace {
	byParent := map[string][]domain.Workspace{}
	for _, w := range all {
		byParent[w.ParentID] = append(byParent[w.ParentID], w)
	}

	seen := map[string]bool{caller.ID: true}
	out := []domain.Workspace{caller}
	queue := []domain.Workspace{caller}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		out, queue = appendUnseenChildren(byParent[cur.ID], seen, out, queue)
	}
	return out
}

func appendUnseenChildren(
	children []domain.Workspace,
	seen map[string]bool,
	out, queue []domain.Workspace,
) ([]domain.Workspace, []domain.Workspace) {
	for _, child := range children {
		if seen[child.ID] {
			continue
		}
		seen[child.ID] = true
		out = append(out, child)
		queue = append(queue, child)
	}
	return out, queue
}

func filter(all []domain.Workspace, keep func(domain.Workspace) bool) []domain.Workspace {
	out := make([]domain.Workspace, 0, len(all))
	for _, w := range all {
		if keep(w) {
			out = append(out, w)
		}
	}
	return out
}
