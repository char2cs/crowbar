package agenttools

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var (
	// ErrUnauthorized means the caller could not be established at all: a bad
	// token, a dead runner, or a runner with no current chat.
	ErrUnauthorized = errors.New("agenttools: unauthorized")
	// ErrOutOfScope means the caller is real but asked about something outside
	// what its position in the workspace tree permits.
	ErrOutOfScope = errors.New("agenttools: out of scope")
	// ErrToolsDisabled means the caller is real and in scope, but the user has
	// switched Crowbar's tool surface off for the provider it runs on.
	//
	// The wording is what the MODEL reads — a tool error rides back as result
	// text — so it names the switch and where it lives rather than reporting a
	// generic refusal the model would retry against.
	ErrToolsDisabled = errors.New(
		"agenttools: Crowbar's tools are switched off for this provider " +
			"(Settings → Providers → Tools); no tool call will be served")
)

// RunnerReader is the narrow read port the resolver needs from the runner
// store: enough to check who a token names and where that runner is placed.
type RunnerReader interface {
	Get(ctx context.Context, runnerID string) (domain.AgentRunner, error)
}

// ChatGetter is the single-chat read port: one chat looked up by id. It is
// declared separately from ChatReader because the resolver authenticates and
// never enumerates — handing it the listing half would give the authority path a
// capability it has no use for.
type ChatGetter interface {
	Get(ctx context.Context, chatID string) (domain.AgentChat, error)
}

// ChatReader is the read port the chat-facing tools need from the chat store:
// one chat by id, plus every chat in ONE read.
//
// It lists the whole table rather than one workspace's chats because
// list_workspaces folds in the chats of every workspace it can see, and the
// store's own ListByWorkspace is a full scan filtered in Go — so asking it once
// per visible workspace read the same table, and unmarshalled every row of it,
// V times over to answer a single question. Bucketing one read by WorkspaceID
// costs one pass instead.
type ChatReader interface {
	ChatGetter
	ListChats(ctx context.Context) ([]domain.AgentChat, error)
}

// WorkspaceLister is the narrow read port the resolver needs from the
// workspace store: one lookup for the caller's own workspace, one full list to
// compute the downward visibility set from.
type WorkspaceLister interface {
	Get(ctx context.Context, wsID string) (domain.Workspace, error)
	List(ctx context.Context) ([]domain.Workspace, error)
}

// Caller is an authenticated agent and everything it is allowed to reach.
//
// ProviderID names the vendor CLI behind this caller (its runner's provider). It
// is carried here so anything an agent WRITES can be attributed to the agent that
// wrote it — a review comment with no author renders as a blank name beside the
// user's own comments in the review UI.
//
// The visible set is reached through Visible and CanSee, never as a field,
// because it is loaded LAZILY: computing it is a full workspace-table scan plus a
// JSON unmarshal per row, and most tools never look at it — set_chat_title needs
// no workspace tree at all, and the review tools need a single membership answer.
// A copy of a Caller shares the same set, so the load happens at most once per
// resolved caller however many times it is passed by value.
type Caller struct {
	RunnerID   string
	ChatID     string
	ProviderID string
	Workspace  domain.Workspace
	visible    *visibleSet
}

// Visible returns every workspace this caller may act on, always including its
// own, loading the set on first use.
//
// It returns an error rather than an empty slice when the set cannot be loaded,
// because to list_workspaces those two are opposite answers: "you can see
// nothing" is a statement a model will act on, and reporting it for what is
// really a failed read would tell an agent its siblings do not exist.
func (c Caller) Visible() ([]domain.Workspace, error) {
	if c.visible == nil {
		return nil, fmt.Errorf("agenttools: visible: caller carries no visibility set: %w", ErrUnauthorized)
	}
	return c.visible.get()
}

// CanSee reports whether wsID is one of the workspaces this caller may act on.
//
// A set that could not be loaded DENIES. This is the authority check every
// cross-workspace tool funnels through — reading another chat's log, replying to
// or resolving another workspace's review thread — so the failure direction is
// not a style choice: treating an unreadable workspace tree as "no restriction"
// would hand an agent every workspace on the machine at the exact moment the
// daemon lost the ability to tell them apart.
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

// visibleSet is one caller's downward visibility, computed at most once and only
// if something asks.
//
// The mutex is not defensive: a Caller is passed by value into a tool handler
// while the pointer to this set is shared with every copy, and nothing stops a
// handler from consulting it off more than one goroutine — a broadcast fan-out
// or a future concurrent tool would do exactly that. The error is memoised
// alongside the set so a caller cannot see a different visible set at two points
// in the same tool call: a check that passes and a later one that fails is
// harder to reason about than either outcome consistently.
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

// Resolver is the authority model: given an authenticated runner, it decides
// which workspaces — and by extension which chats — that runner may reach.
// Every agent tool goes through it so authorization happens in exactly one
// place.
type Resolver struct {
	minter     *TokenMinter
	runners    RunnerReader
	chats      ChatGetter
	workspaces WorkspaceLister
}

// NewResolver wires a Resolver to the token minter and the three read-only
// ports it consults to turn a (runnerID, token) pair into a Caller.
func NewResolver(
	minter *TokenMinter,
	runners RunnerReader,
	chats ChatGetter,
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
	return Caller{
		RunnerID:   runnerID,
		ChatID:     runner.CurrentChatID,
		ProviderID: runner.ProviderID,
		Workspace:  ws,
		visible:    &visibleSet{load: r.visibleLoader(ctx, ws)},
	}, nil
}

// visibleLoader defers the workspace-tree read until a tool actually needs it.
//
// Resolve runs on EVERY MCP call, and the read it used to do here is a full
// table scan with a JSON unmarshal per row — paid by set_chat_title, which never
// looks at a workspace other than its own, as much as by list_workspaces, which
// is the only tool that wants the whole tree.
//
// It closes over Resolve's context deliberately: ToolSet.Call hands the same
// context to Resolve and to the tool handler it dispatches, so the deferred read
// runs under the request that asked for it and is cancelled with it. Binding the
// context here rather than taking one per call is what keeps CanSee's signature
// free of a context it would only ever be given the same value of.
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

// visibleFrom applies the three-tier rule and is downward only by
// construction: none of its branches ever consult a workspace's ancestors.
//
// It drops deleted workspaces first, so all three branches inherit the filter.
// WorkspaceLister.List returns the residual rows of workspaces the user has
// already deleted, and every other consumer of that list skips them (the
// branch-taken checks in the workspaces handler and in worktree, and merge
// eligibility). Without this the tool surface would be the one place that does
// not: list_workspaces would advertise workspaces the user's own sidebar no
// longer shows, and the tools reached through CanSee would act on them.
//
// The caller's own workspace is kept regardless, because Caller.Visible always
// containing it is an invariant the tools rely on — a runner whose workspace is
// mid-delete must still resolve to itself rather than to nothing.
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
