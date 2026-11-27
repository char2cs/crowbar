//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
)

const stubProviderDescriptorYAML = `id: stub
spawn:
  cmd: "true"
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
surfaces:
  chat:     { channel: hooks, start_here: true }
  terminal: { channel: hooks, start_here: true }
`

func writeStubProviderDescriptor(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	writeProviderDescriptor(t, h, "stub", stubProviderDescriptorYAML)
}

type agentChatDTO struct {
	ID                string `json:"id"`
	WorkspaceID       string `json:"workspaceId"`
	Title             string `json:"title"`
	LiveRunnerID      string `json:"liveRunnerId"`
	TerminalSessionID string `json:"terminalSessionId"`
	ActiveProviderID  string `json:"activeProviderId"`
	ParentID          string `json:"parentId"`
	Order             int    `json:"order"`
	Type              string `json:"type"`
	Worktree          *struct {
		OwningChatID string `json:"owningChatId"`
	} `json:"worktree"`
}

func conversationsOnly(
	rows []agentChatDTO,
) []agentChatDTO {
	out := make([]agentChatDTO, 0, len(rows))
	for _, row := range rows {
		if row.Type != "chat" {
			continue
		}
		if row.Worktree != nil && row.Worktree.OwningChatID == row.ID {
			continue
		}
		out = append(out, row)
	}
	return out
}

type agentChatConversation struct {
	ChatID     string `json:"chatId"`
	ProviderID string `json:"providerId"`
	SessionID  string `json:"sessionId"`
}

type agentChatDetail struct {
	agentChatDTO
	Conversations []agentChatConversation `json:"conversations"`
}

func getAgentChat(
	t *testing.T,
	h *harness,
	base string,
	chatID string,
) agentChatDetail {
	t.Helper()
	var detail agentChatDetail
	h.get(base+"/chats/"+chatID, &detail)
	return detail
}

func (d agentChatDetail) sessionIDs() []string {
	out := make([]string, 0, len(d.Conversations))
	for _, c := range d.Conversations {
		out = append(out, c.SessionID)
	}
	return out
}

func createAgentChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	h.QuiesceReactors()
	return created.ID
}

func TestAgentREST_Scope(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

	projA := importProject(t, h)
	projB := importProject(t, h)
	homeA := "/v0/projects/" + projA.projectID + "/home"
	homeB := "/v0/projects/" + projB.projectID + "/home"

	var homeChatA struct {
		ID string `json:"id"`
	}
	h.post(homeA+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &homeChatA)
	require.NotEmpty(t, homeChatA.ID)
	h.QuiesceReactors()

	var homeChatB struct {
		ID string `json:"id"`
	}
	h.post(homeB+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &homeChatB)
	require.NotEmpty(t, homeChatB.ID)
	h.QuiesceReactors()

	var listA []agentChatDTO
	h.get(homeA+"/chats", &listA)
	listA = conversationsOnly(listA)
	var sawA, sawB bool
	for _, row := range listA {
		switch row.ID {
		case homeChatA.ID:
			sawA = true
		case homeChatB.ID:
			sawB = true
		}
	}
	assert.True(t, sawA, "project A's home chat list must contain its own chat")
	assert.False(t, sawB, "project A's home chat list must never contain B's chat")

	homeResp := h.raw(http.MethodGet, homeA+"/chats/"+homeChatB.ID, nil, http.StatusNotFound)
	_ = homeResp.Body.Close()

	var gotHomeB agentChatDTO
	h.get(homeB+"/chats/"+homeChatB.ID, &gotHomeB)
	assert.Equal(t, homeChatB.ID, gotHomeB.ID, "sanity: B's own home mount still resolves its own chat")

	imported := importProject(t, h)
	mainWS := imported.workspaceID
	otherWS := createChildWorkspace(t, h, imported, "feature/other", mainWS)
	h.Quiesce()

	chatMain := createAgentChat(t, h, imported)
	other := imported
	other.workspaceID = otherWS
	chatOther := createAgentChat(t, h, other)

	base := repoBase(imported)
	var list []agentChatDTO
	h.get(base+"/chats", &list)
	ids := map[string]bool{}
	for _, c := range list {
		ids[c.ID] = true
	}
	assert.True(t, ids[chatMain], "the repo-scoped list carries the main workspace's own chat")
	assert.True(t, ids[chatOther], "and the repo's OTHER workspace's chat too — one list, whole repo")
	assert.False(t, ids[homeChatA.ID],
		"another project's home chat must not appear in this repo's list")
	assert.False(t, ids[homeChatB.ID],
		"nor the second project's — a repo-scoped list is scoped to the repo")

	var gotMain agentChatDTO
	h.get(base+"/chats/"+chatMain, &gotMain)
	assert.Equal(t, mainWS, gotMain.WorkspaceID)

	var gotOther agentChatDTO
	h.get(base+"/chats/"+chatOther, &gotOther)
	assert.Equal(t, otherWS, gotOther.WorkspaceID,
		"a chat born in a DIFFERENT workspace of this repo is still reachable through the repo's own "+
			"mount: there is no :wsId segment left for the URL to get wrong")
}

func spawnStubChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
) (int, string, string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"provider": "stub", "workspaceId": imported.workspaceID, "surface": "terminal"})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, h.url+repoBase(imported)+"/chats", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var env struct {
		Success bool            `json:"success"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	if !env.Success {
		require.NotEmpty(t, env.Error, "error envelope must carry a message")
		return resp.StatusCode, env.Error, ""
	}
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &created))
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	return resp.StatusCode, "", created.ID
}

func TestRegression_ProviderThatExitsDuringStartupIsRefused(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	frames := recordSidebar(t, h)

	status, msg, chatID := spawnStubChat(t, h, ws)
	h.QuiesceReactors()

	if status == http.StatusFailedDependency {
		assert.Contains(t, msg, "exited during startup",
			"the refusal has to name what went wrong: the user's own CLI died, and only they can fix it")
		return
	}

	require.Equal(t, http.StatusCreated, status,
		"a CLI that exits during startup is either refused as a dependency failure or "+
			"accepted as a chat that is already dormant — never any other status")
	frames.awaitAfter(chatID, "dormant once its CLI died",
		func(chat map[string]any) bool { return chat["liveRunnerId"] != "" },
		func(chat map[string]any) bool { return chat["liveRunnerId"] == "" && chat["terminalSessionId"] == "" })
	var chat agentChatDTO
	h.get(repoBase(ws)+"/chats/"+chatID, &chat)
	assert.Empty(t, chat.LiveRunnerID,
		"the CLI is dead, so the chat it was handed back for must read as dormant")
	assert.Empty(t, chat.TerminalSessionID,
		"and must not point a pane at a PTY that is already gone")
}

func TestRegression_RefusedSpawnLeavesNoChatBehind(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)

	status, _, refusedChatID := spawnStubChat(t, h, ws)
	h.QuiesceReactors()

	wantIDs := []string{}
	if status == http.StatusCreated {
		wantIDs = []string{refusedChatID}
	}

	require.Equal(t, wantIDs, chatIDs(t, h, repoBase(ws)),
		"the chat list must agree with the answer the API gave: a refusal leaves nothing behind")

	chatsDir, err := h.app.Usecases.AgentWorkspaceReader.AgentChatsDir(context.Background(), ws.workspaceID)
	require.NoError(t, err)
	for _, dir := range []string{chatsDir, worktreepath.LedgerChatsDir(h.home)} {
		entries, err := os.ReadDir(dir)
		require.Truef(t, err == nil || os.IsNotExist(err), "read %s: %v", dir, err)
		for _, entry := range entries {
			assert.Contains(t, append([]string{"runners"}, wantIDs...), entry.Name(), "a refused spawn left a chat dir behind")
		}
	}

	created := createAgentChat(t, h, ws)
	require.Equal(t, append(wantIDs, created), chatIDs(t, h, repoBase(ws)),
		"the refusal cost the workspace nothing: a working provider still creates one chat")
}
