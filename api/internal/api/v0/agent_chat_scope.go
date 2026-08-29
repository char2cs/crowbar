package v0

import (
	"context"
	"sync"
)

// Which repo an agent-chat frame belongs to, and how that answer is kept cheap.
//
// A chat row carries no repo id — a row's repo is the repo of the workspace its
// cwd walk lands on (model spec §3.2), which is derived and never stored — and
// a BUBBLE carries no workspace either. So every frame resolves its own repo:
// from the workspace it names, or, for a bubble, from the workspace its walk
// lands on. A row that resolves neither has no repo to be held to and says so
// with the empty string (see matchRepoOrUnscoped).
//
// A workspace's owning repo never changes, so that half is memoized outright.
// The walk's half CAN change — a drag re-parents a bubble under a row in
// another repo — so it is memoized only until the next STRUCTURAL frame, at
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
	byChat      map[string]string
	byWorkspace map[string]string
}

func newAgentChatScopes() *agentChatScopes {
	return &agentChatScopes{
		byChat:      map[string]string{},
		byWorkspace: map[string]string{},
	}
}

// forget drops every walked answer, which the next frame re-resolves.
func (s *agentChatScopes) forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byChat = map[string]string{}
}

func (s *agentChatScopes) chat(
	chatID string,
) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	repoID, ok := s.byChat[chatID]
	return repoID, ok
}

func (s *agentChatScopes) rememberChat(
	chatID string,
	repoID string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byChat[chatID] = repoID
}

func (s *agentChatScopes) workspace(
	workspaceID string,
) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	repoID, ok := s.byWorkspace[workspaceID]
	return repoID, ok
}

func (s *agentChatScopes) rememberWorkspace(
	workspaceID string,
	repoID string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byWorkspace[workspaceID] = repoID
}

// agentChatRepo answers the repo one frame belongs to, reading the memo for a
// bubble rather than walking the forest again.
func (c *Container) agentChatRepo(
	chatID string,
	workspaceID string,
) string {
	if workspaceID != "" {
		return c.workspaceRepo(workspaceID)
	}
	if repoID, ok := c.chatScopes.chat(chatID); ok {
		return repoID
	}
	return c.walkChatRepo(chatID)
}

// freshAgentChatRepo is agentChatRepo for a STRUCTURAL frame — a create, a
// placement, a delete, a runner move — which is exactly the kind of change that
// can have moved a bubble into another repo. It drops the memo first, so the
// answer it computes is the one after the change rather than the one before.
func (c *Container) freshAgentChatRepo(
	chatID string,
	workspaceID string,
) string {
	c.chatScopes.forget()
	return c.agentChatRepo(chatID, workspaceID)
}

// walkChatRepo resolves a bubble's repo through the chat usecase's own cwd walk
// and memoizes what it found. Only a real answer is memoized: a row that
// resolves nothing may be one whose placement the projection has not caught up
// with yet, and caching that would leave it unscoped until the next structural
// frame arrived to clear it.
func (c *Container) walkChatRepo(
	chatID string,
) string {
	if c.app == nil || c.app.Usecases == nil || c.app.Usecases.AgentChat == nil {
		return ""
	}
	workspaceID, ok, err := c.app.Usecases.AgentChat.CwdWorkspaceID(context.Background(), chatID)
	if err != nil || !ok {
		return ""
	}
	repoID := c.workspaceRepo(workspaceID)
	if repoID != "" {
		c.chatScopes.rememberChat(chatID, repoID)
	}
	return repoID
}

// workspaceRepo memoizes one workspace's owning repo. It is written at creation
// and never moves, so a real answer cannot go stale and is never invalidated;
// an unresolvable one is not memoized at all, since a read taken before the
// workspace's projection caught up would otherwise be wrong for the life of the
// daemon.
func (c *Container) workspaceRepo(
	workspaceID string,
) string {
	if repoID, ok := c.chatScopes.workspace(workspaceID); ok {
		return repoID
	}
	_, repoID := c.resolveWorkspaceScope(context.Background(), workspaceID)
	if repoID != "" {
		c.chatScopes.rememberWorkspace(workspaceID, repoID)
	}
	return repoID
}
