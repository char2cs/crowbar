package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// compactAPITestDescriptor mirrors interruptTestDescriptor's shape (same
// package, same "acme" fake CLI convention) with a compact_start gesture
// whose wire is an api-transport call, not "prompt" — the shape codex.yaml
// itself declares (thread/compact/start), and the one Compact() used to
// refuse outright.
const compactAPITestDescriptor = `
id: compact-api-test
spawn:
  cmd: acme
  interactive_required: true
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
  turn_stop:
    in: turn/completed
    map:
      session_id: threadId
      message: "turn.items[type=agentMessage].text"
  compact_start:
    out: thread/compact/start
    send: { threadId: "{session_id}" }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [acme, serve]
    handshake: { call: initialize }
`

func compactAPITestAgent(t *testing.T) engineagents.Agent {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "compact-api-test.yaml"), []byte(compactAPITestDescriptor), 0o600))
	a, err := engineagents.New().Get(context.Background(), home, "compact-api-test")
	require.NoError(t, err)
	return a
}

// stubChatsForCompact answers only GetChat — Compact's first call, to learn
// the chat's WorkspaceID.
type stubChatsForCompact struct {
	agentchat.EventStore
	chat domain.Chat
}

func (s stubChatsForCompact) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

// stubConversationsForCompact answers only ChatProviderID — Compact's second
// call, to resolve which descriptor to ask.
type stubConversationsForCompact struct {
	Conversations
	providerID string
}

func (s stubConversationsForCompact) ChatProviderID(context.Context, string) (string, error) {
	return s.providerID, nil
}

// TestCompact_APITransport_SendsThreadCompactStart is Compact()'s new path,
// end to end: a provider whose compact_start declares an api-transport call
// (codex: thread/compact/start, confirmed live against codex-cli 0.149.1)
// now gets it actually SENT over the chat's live connection, exactly the way
// interruptTurn already drives turn/interrupt. Before this fix, Compact()
// refused any wire other than "prompt" outright — codex's own compact button
// never worked at all, which meant compact_pre/compact_post's new live
// mapping (item/started/completed, item.type: contextCompaction) had nothing
// to actually observe outside of codex's own automatic compaction.
func TestCompact_APITransport_SendsThreadCompactStart(t *testing.T) {
	received := make(chan string, 1)
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage() // thread/compact/start
		require.NoError(t, err)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		received <- req.Method + ":" + string(req.Params)
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	agent := compactAPITestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		chats:         stubChatsForCompact{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}},
		conversations: stubConversationsForCompact{providerID: "compact-api-test"},
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "compact-api-test"},
		},
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})

	require.NoError(t, rs.Compact(ctx, "chat-1"))

	select {
	case got := <-received:
		require.Contains(t, got, "thread/compact/start:")
	case <-time.After(3 * time.Second):
		t.Fatal("the socket never received thread/compact/start")
	}
}

// TestCompact_APITransport_NoLiveConnIsUnavailable is the refusal Compact()
// still owes a chat with no live api connection to ask — still spawning,
// running over hooks-only, or between runners — rather than silently doing
// nothing or dialing a fresh connection with no session to compact.
func TestCompact_APITransport_NoLiveConnIsUnavailable(t *testing.T) {
	agent := compactAPITestAgent(t)
	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		chats:         stubChatsForCompact{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}},
		conversations: stubConversationsForCompact{providerID: "compact-api-test"},
		runnerStore: stubRunnerStoreForAttach{
			runner: engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "compact-api-test"},
		},
	}

	err := rs.Compact(context.Background(), "chat-1")
	require.Error(t, err)
	require.ErrorIs(t, err, apperr.ErrUnavailable)
}
