package v0

import (
	"context"
	"sync"
)

// Which PROJECT and repo an agent-chat frame belongs to, and how that answer is
// kept cheap.
//
// A chat row carries no repo id — a row's repo is the repo of the workspace its
// cwd walk lands on (model spec §3.2), which is derived and never stored — and
// a BUBBLE carries no workspace either. So every frame resolves its own scope:
// from the workspace it names, or, for a bubble, from the workspace its walk
// lands on. A row that resolves neither has no scope to be held to and says so
// with empty strings (see matchScopeOrUnscoped).
//
// The pair is resolved together, from the SAME workspace row, because neither
// half alone scopes this feed. Repo alone cannot: a PROJECT-HOME workspace owns
// no repo, so every frame a home chat emits — including a turn's streamed text —
// is repo-less, and a repo-less frame is deliberately forwarded to every
// repo-scoped subscriber so folder rows and root bubbles keep repainting. Before
// the project half existed that forwarding had no boundary at all, and a home
// chat's frames crossed into every repo of every project in the process.
//
// A workspace's owning project and repo never change, so that half is memoized
// outright. The walk's half CAN change — a drag re-parents a bubble under a row
// in another repo — so it is memoized only until the next STRUCTURAL frame, at
// which point the whole map is dropped. Structural frames are rare; the frames
// that are not (a streaming message's deltas, a terminal-wait edge, at the
// detector's 2s cadence) are exactly the ones that must not pay for a walk, and
// they are the ones that read the memo.
//
// Dropping the whole map rather than one row's entry is deliberate: a bubble's
// repo depends on its ANCESTORS, so the move that changes it may announce a
// different row entirely. Invalidating precisely would need the forest this
// type exists to avoid reading.
type agentChatScopes struct {
	mu          sync.Mutex
	byChat      map[string]chatScope
	byWorkspace map[string]chatScope
}

// chatScope is the (project, repo) pair one frame is held to.
//
// An empty RepoID with a real ProjectID is a WHOLE answer, not a partial one: it
// is exactly what a project-home row is. An empty ProjectID means nothing about
// the row resolved at all.
type chatScope struct {
	ProjectID string
	RepoID    string
}

// resolved reports whether this scope answers anything. ProjectID is the test
// rather than RepoID, because a home workspace legitimately has no repo — keying
// the memo on RepoID would re-resolve every home frame forever AND would treat a
// real answer as a failure.
func (s chatScope) resolved() bool {
	return s.ProjectID != ""
}

func newAgentChatScopes() *agentChatScopes {
	return &agentChatScopes{
		byChat:      map[string]chatScope{},
		byWorkspace: map[string]chatScope{},
	}
}

// forget drops every walked answer, which the next frame re-resolves.
func (s *agentChatScopes) forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byChat = map[string]chatScope{}
}

func (s *agentChatScopes) chat(
	chatID string,
) (chatScope, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, ok := s.byChat[chatID]
	return scope, ok
}

func (s *agentChatScopes) rememberChat(
	chatID string,
	scope chatScope,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byChat[chatID] = scope
}

func (s *agentChatScopes) workspace(
	workspaceID string,
) (chatScope, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scope, ok := s.byWorkspace[workspaceID]
	return scope, ok
}

func (s *agentChatScopes) rememberWorkspace(
	workspaceID string,
	scope chatScope,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byWorkspace[workspaceID] = scope
}

// agentChatScope answers the project and repo one frame belongs to, reading the
// memo for a bubble rather than walking the forest again.
func (c *Container) agentChatScope(
	chatID string,
	workspaceID string,
) chatScope {
	if workspaceID != "" {
		return c.workspaceScope(workspaceID)
	}
	if scope, ok := c.chatScopes.chat(chatID); ok {
		return scope
	}
	return c.walkChatScope(chatID)
}

// freshAgentChatScope is agentChatScope for a STRUCTURAL frame — a create, a
// placement, a delete, a runner move — which is exactly the kind of change that
// can have moved a bubble into another repo. It drops the memo first, so the
// answer it computes is the one after the change rather than the one before.
func (c *Container) freshAgentChatScope(
	chatID string,
	workspaceID string,
) chatScope {
	c.chatScopes.forget()
	return c.agentChatScope(chatID, workspaceID)
}

// walkChatScope resolves a bubble's scope through the chat usecase's own cwd
// walk and memoizes what it found. Only a real answer is memoized: a row that
// resolves nothing may be one whose placement the projection has not caught up
// with yet, and caching that would leave it unscoped until the next structural
// frame arrived to clear it.
func (c *Container) walkChatScope(
	chatID string,
) chatScope {
	if c.app == nil || c.app.Usecases == nil || c.app.Usecases.AgentChat == nil {
		return chatScope{}
	}
	workspaceID, ok, err := c.app.Usecases.AgentChat.CwdWorkspaceID(context.Background(), chatID)
	if err != nil || !ok {
		return chatScope{}
	}
	scope := c.workspaceScope(workspaceID)
	if scope.resolved() {
		c.chatScopes.rememberChat(chatID, scope)
	}
	return scope
}

// workspaceScope memoizes one workspace's owning project and repo. Both are
// written at creation and never move, so a real answer cannot go stale and is
// never invalidated; an unresolvable one is not memoized at all, since a read
// taken before the workspace's projection caught up would otherwise be wrong for
// the life of the daemon.
func (c *Container) workspaceScope(
	workspaceID string,
) chatScope {
	if scope, ok := c.chatScopes.workspace(workspaceID); ok {
		return scope
	}
	projectID, repoID := c.resolveWorkspaceScope(context.Background(), workspaceID)
	scope := chatScope{ProjectID: projectID, RepoID: repoID}
	if scope.resolved() {
		c.chatScopes.rememberWorkspace(workspaceID, scope)
	}
	return scope
}
