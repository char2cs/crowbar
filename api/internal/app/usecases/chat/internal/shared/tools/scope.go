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
	// ErrUnauthorized means the caller could not be established at all: a bad
	// token, a dead runner, or a runner with no current chat.
	ErrUnauthorized = problem.New(problem.Forbidden, "agenttools: unauthorized")
	// ErrOutOfScope means the caller is real but asked about something outside
	// what its position in the workspace tree permits.
	ErrOutOfScope = errors.New("agenttools: out of scope")
	// ErrOwnThread means the caller asked for a chat below its own in the Chats tree. Threads already read
	// their parent, so reading down would merge independent attempts into one conversation. The model
	// reads this text, so it names the closed direction instead of a generic refusal.
	ErrOwnThread = errors.New(
		"agenttools: that chat is a thread of this one; a chat reads the chats ABOVE it, never below",
	)
	// ErrToolsDisabled means the user switched Crowbar's tools off for the caller's provider. The model
	// reads this text, so it names the switch rather than a refusal it would retry.
	ErrToolsDisabled = errors.New(
		"agenttools: Crowbar's tools are switched off for this provider " +
			"(Settings → Providers → Tools); no tool call will be served",
	)
)

// RunnerReader is the narrow read port the resolver needs from the runner
// supervisor: the live runner a token names, and so the chat it is on.
type RunnerReader interface {
	RunnerByID(runnerID string) (agents.Runner, bool)
}

// ChatReader is the chat store's read port: one chat by id, or every chat in one read. list_workspaces
// buckets a single full read by workspace; per-workspace listing would rescan the table each time.
type ChatReader interface {
	Get(ctx context.Context, chatID string) (domain.Chat, error)
	ListChats(ctx context.Context) ([]domain.Chat, error)
}

// WorkspaceLister is the narrow read port the resolver needs from the
// workspace store: one lookup for the caller's own workspace, one full list to
// compute the downward visibility set from.
type WorkspaceLister interface {
	Get(ctx context.Context, wsID string) (domain.Workspace, error)
	List(ctx context.Context) ([]domain.Workspace, error)
}

// Caller is an authenticated agent and everything it may reach. ProviderID attributes what it writes.
// The visible set is loaded lazily (a full workspace scan most tools never need) and shared by every
// copy, so it loads at most once per resolved caller.
type Caller struct {
	RunnerID   string
	ChatID     string
	ProviderID string
	Workspace  domain.Workspace
	visible    *visibleSet
}

// Visible returns every workspace this caller may act on, always including its own, loading the set on
// first use. A failed load is an error, never an empty slice: "you can see nothing" is an answer.
func (c Caller) Visible() ([]domain.Workspace, error) {
	if c.visible == nil {
		return nil, fmt.Errorf("agenttools: visible: caller carries no visibility set: %w", ErrUnauthorized)
	}
	return c.visible.get()
}

// CanSee reports whether wsID is one of the workspaces this caller may act on. Every cross-workspace
// tool funnels through it, so a set that could not be loaded denies.
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

// visibleSet is one caller's downward visibility, computed at most once and only if asked. Copies of a
// Caller share it across goroutines, hence the mutex; the error is memoised too so one call never sees
// two different answers.
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

// Resolver is the authority model: it decides which workspaces (and so chats) an authenticated runner
// may reach. Every agent tool goes through it, so authorization lives in one place.
type Resolver struct {
	minter     *TokenMinter
	runners    RunnerReader
	workspaces WorkspaceLister
}

// NewResolver wires a Resolver to the token minter and the two read-only
// ports it consults to turn a (runnerID, token) pair into a Caller.
func NewResolver(
	minter *TokenMinter,
	runners RunnerReader,
	workspaces WorkspaceLister,
) *Resolver {
	return &Resolver{minter: minter, runners: runners, workspaces: workspaces}
}

// Resolve authenticates (runnerID, token) against the live runner table, so an exited runner resolves
// to nothing.
func (r *Resolver) Resolve(ctx context.Context, runnerID, token string) (Caller, error) {
	// A resolver missing a port authenticates nobody, so it refuses everybody:
	// a misconfiguration fails CLOSED, never crashes on the first call.
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
	ws, err := r.workspaces.Get(ctx, runner.WorkspaceID)
	if err != nil {
		return Caller{}, fmt.Errorf("agenttools: get caller workspace: %w", errors.Join(ErrUnauthorized, err))
	}
	return Caller{
		RunnerID:   runnerID,
		ChatID:     runner.CurrentChatID,
		ProviderID: runner.ProviderID,
		Workspace:  ws,
		visible:    &visibleSet{load: r.visibleLoader(ctx, ws)},
	}, nil
}

// visibleLoader defers the full workspace scan until a tool needs it, since Resolve runs on every MCP
// call. It closes over Resolve's context, which is the same request context the handler runs under.
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

// visibleFrom applies the three-tier rule, downward only: no branch consults ancestors. Deleted
// workspaces (still returned by List) are dropped first, except the caller's own, which Visible always
// contains even mid-delete.
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

// descendants walks the ParentID tree downward from caller, breadth-first; the seen set makes it
// terminate even on a cyclic parent chain.
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
