package agenttools

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var (
	// ErrUnauthorized means the caller could not be established at all: a bad
	// token, a dead runner, or a runner with no current chat.
	ErrUnauthorized = errors.New("agenttools: unauthorized")
	// ErrOutOfScope means the caller is real but asked about something outside
	// what its position in the workspace tree permits.
	ErrOutOfScope = errors.New("agenttools: out of scope")
)

// RunnerReader is the narrow read port the resolver needs from the runner
// store: enough to check who a token names and where that runner is placed.
type RunnerReader interface {
	Get(ctx context.Context, runnerID string) (domain.AgentRunner, error)
}

// ChatReader is the narrow read port the resolver, and the chat-facing tools
// built on top of it, need from the chat store.
type ChatReader interface {
	Get(ctx context.Context, chatID string) (domain.AgentChat, error)
	ListByWorkspace(ctx context.Context, wsID string) ([]domain.AgentChat, error)
}

// WorkspaceLister is the narrow read port the resolver needs from the
// workspace store: one lookup for the caller's own workspace, one full list to
// compute the downward visibility set from.
type WorkspaceLister interface {
	Get(ctx context.Context, wsID string) (domain.Workspace, error)
	List(ctx context.Context) ([]domain.Workspace, error)
}

// Caller is an authenticated agent and everything it is allowed to reach.
// Visible always contains the caller's own workspace.
type Caller struct {
	RunnerID  string
	ChatID    string
	Workspace domain.Workspace
	Visible   []domain.Workspace
}

// CanSee reports whether wsID is one of the workspaces this caller may act on.
func (c Caller) CanSee(wsID string) bool {
	for _, w := range c.Visible {
		if w.ID == wsID {
			return true
		}
	}
	return false
}

// Resolver is the authority model: given an authenticated runner, it decides
// which workspaces — and by extension which chats — that runner may reach.
// Every agent tool goes through it so authorization happens in exactly one
// place.
type Resolver struct {
	minter     *TokenMinter
	runners    RunnerReader
	chats      ChatReader
	workspaces WorkspaceLister
}

// NewResolver wires a Resolver to the token minter and the three read-only
// ports it consults to turn a (runnerID, token) pair into a Caller.
func NewResolver(
	minter *TokenMinter,
	runners RunnerReader,
	chats ChatReader,
	workspaces WorkspaceLister,
) *Resolver {
	return &Resolver{minter: minter, runners: runners, chats: chats, workspaces: workspaces}
}

// Resolve authenticates (runnerID, token) and computes what that caller may
// see.
//
// It resolves through the runner's CURRENT chat rather than a chat id baked in
// at spawn: an agent that clears its conversation moves to a different chat
// while the runner id stays stable, so anything keyed on a baked-in chat id
// would act on the chat the agent used to be on instead of the one it is
// actually on. This is the same property (*agent.Usecase).RenameByRunner
// relies on. A runner with no current chat has been displaced — evicted from
// the chat it was on, dying but not yet reaped — and must not resolve to
// anything.
func (r *Resolver) Resolve(ctx context.Context, runnerID, token string) (Caller, error) {
	// A resolver with no minter can authenticate nobody, so it must refuse
	// everybody. Without this it would instead panic inside Verify — and a
	// misconfiguration must fail CLOSED, never crash the daemon on the first
	// unauthenticated call an agent makes.
	if r.minter == nil {
		return Caller{}, fmt.Errorf("agenttools: resolve: no token minter: %w", ErrUnauthorized)
	}
	if runnerID == "" || !r.minter.Verify(runnerID, token) {
		return Caller{}, ErrUnauthorized
	}
	runner, err := r.runners.Get(ctx, runnerID)
	if err != nil {
		return Caller{}, fmt.Errorf("agenttools: get runner: %w", errors.Join(ErrUnauthorized, err))
	}
	if runner.CurrentChatID == "" {
		return Caller{}, fmt.Errorf("agenttools: resolve runner: %w", ErrUnauthorized)
	}
	ws, err := r.workspaces.Get(ctx, runner.WorkspaceID)
	if err != nil {
		return Caller{}, fmt.Errorf("agenttools: get caller workspace: %w", errors.Join(ErrUnauthorized, err))
	}
	all, err := r.workspaces.List(ctx)
	if err != nil {
		return Caller{}, fmt.Errorf("agenttools: list workspaces: %w", err)
	}
	return Caller{
		RunnerID:  runnerID,
		ChatID:    runner.CurrentChatID,
		Workspace: ws,
		Visible:   visibleFrom(ws, all),
	}, nil
}

// visibleFrom applies the three-tier rule and is downward only by
// construction: none of its branches ever consult a workspace's ancestors.
func visibleFrom(caller domain.Workspace, all []domain.Workspace) []domain.Workspace {
	switch {
	case caller.Kind == domain.WorkspaceKindHome:
		return filter(all, func(w domain.Workspace) bool { return w.ProjectID == caller.ProjectID })
	case caller.IsDefault:
		return filter(all, func(w domain.Workspace) bool {
			return w.ProjectID == caller.ProjectID && w.RepoID == caller.RepoID
		})
	default:
		return descendants(caller, all)
	}
}

// descendants walks the ParentID tree downward from caller, breadth-first.
// The seen set guards against a cycle in the parent chain — which a bug
// elsewhere could produce — so the walk always terminates instead of spinning
// forever.
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
