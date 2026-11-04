//go:build integration

package kit

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// DialTerminalPTY watches the raw PTY stream, a direct engine pipe rather than a broadcaster
// topic, so there is no WaitNRegistered gate.
func (e *Env) DialTerminalPTY(
	t *testing.T,
	chatID string,
	sessionID string,
) *WSWatcher {
	t.Helper()
	return Dial(t, e.dialer, wsURL(e.URL, e.chatScope(chatID)+"/terminals/"+sessionID+"/ws"))
}

// chatScope builds the FLAT chat path prefix shared by all chat-scoped routes.
// Chat ids are globally unique, so nothing below it re-nests under a
// project/repo pair.
func (e *Env) chatScope(
	chatID string,
) string {
	return "/v0/chats/" + chatID
}

// OwningChatID returns the chat that OWNS the workspace (minted chat-first, so it already exists).
// It reads in-process because workspaces are no longer addressable over HTTP, using the same
// ListChatsByWorkspace + domain.ResolveOwningChat calls as the wire resolver.
func (e *Env) OwningChatID(
	t *testing.T,
	wsID string,
) string {
	t.Helper()
	ctx := context.Background()
	// The chat-first mint is an asynx write; drain the projections so the read
	// below sees the row rather than racing it (no polling, no sleep).
	e.Quiesce()

	rows, err := e.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, wsID)
	require.NoError(t, err, "OwningChatID: list the chats holding %s", wsID)
	owner, ok := domain.ResolveOwningChat(rows)
	require.Truef(
		t,
		ok,
		"workspace %s must be held by an owning chat for the chat-scoped routes",
		wsID,
	)
	return owner.ID
}

// ListChats reads a repo's chat rows over REST: each worktree-owning row carries its git
// state inline.
func (e *Env) ListChats(
	t *testing.T,
	projectID string,
	repoID string,
) []map[string]any {
	t.Helper()
	resp := e.GET(t, "/v0/projects/"+projectID+"/repos/"+repoID+"/chats") //nolint:bodyclose // DecodeEnvData closes the body
	RequireStatus(t, resp, http.StatusOK)
	var chats []map[string]any
	DecodeEnvData(t, resp, &chats)
	return chats
}

// WorktreeChats returns the repo's chat rows that OWN a worktree, projected to
// the flat workspace shape (WorktreeFrame's REST twin), keyed by workspace id.
// It is the read-model replacement for the deleted workspace list.
func (e *Env) WorktreeChats(
	t *testing.T,
	projectID string,
	repoID string,
) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, c := range e.ListChats(t, projectID, repoID) {
		wsID, _ := c["workspaceId"].(string)
		wt, ok := c["worktree"].(map[string]any)
		if wsID == "" || !ok {
			continue
		}
		flat := make(map[string]any, len(wt)+2)
		for k, v := range wt {
			flat[k] = v
		}
		flat["id"] = wsID
		flat["chatId"] = c["id"]
		out[wsID] = flat
	}
	return out
}
