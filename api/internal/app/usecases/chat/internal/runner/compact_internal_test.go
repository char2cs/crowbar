package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

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
    delivery: stdio
    format: ndjson
    serve: [acme, serve]
    handshake: { call: initialize, notify: initialized }
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

type stubChatsForCompact struct {
	agentchat.EventStore
	chat domain.Chat
}

func (s stubChatsForCompact) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

type stubConversationsForCompact struct {
	Conversations
	providerID string
}

func (s stubConversationsForCompact) ChatProviderID(context.Context, string) (string, error) {
	return s.providerID, nil
}

func TestCompact_APITransport_SendsThreadCompactStart(t *testing.T) {
	received := make(chan string, 1)
	stdio := fakeServe(t, func(conn *testutil.LinePeer) {
		msg, err := conn.Read()
		require.NoError(t, err)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		received <- req.Method + ":" + string(req.Params)
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{}})
		require.NoError(t, conn.Write(resp))
		_, _ = conn.Read()
	})

	agent := compactAPITestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = apiConn.Close() }()

	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		chats:         stubChatsForCompact{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}},
		conversations: stubConversationsForCompact{providerID: "compact-api-test"},
		live:          liveWith(t, engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "compact-api-test", CurrentChatID: "chat-1"}),
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})

	require.NoError(t, rs.Compact(ctx, "chat-1"))

	select {
	case got := <-received:
		require.Contains(t, got, "thread/compact/start:")
	case <-time.After(3 * time.Second):
		t.Fatal("serve never received thread/compact/start")
	}
}

func TestCompact_APITransport_NoLiveConnIsUnavailable(t *testing.T) {
	agent := compactAPITestAgent(t)
	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		chats:         stubChatsForCompact{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}},
		conversations: stubConversationsForCompact{providerID: "compact-api-test"},
		live:          liveWith(t, engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "compact-api-test", CurrentChatID: "chat-1"}),
	}

	err := rs.Compact(context.Background(), "chat-1")
	require.Error(t, err)
	require.ErrorIs(t, err, problem.ErrUnavailable)
}

func TestRegression_CompactIsRefusedOnTheProvidersOwnTerminal(t *testing.T) {
	agent := compactAPITestAgent(t)
	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		chats:         stubChatsForCompact{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1", Surface: domain.SurfaceTerminal}},
		conversations: stubConversationsForCompact{providerID: "compact-api-test"},
	}

	err := rs.Compact(context.Background(), "chat-1")
	require.ErrorIs(t, err, ErrCompactionOffSurface)
}

func TestCompact_IsOfferedOnCrowbarsOwnChatSurface(t *testing.T) {
	agent := compactAPITestAgent(t)
	rs := &Runners{
		apiConns:      newAPIConnRegistry(),
		ws:            stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents:        stubAgentsForInterrupt{agent: agent},
		chats:         stubChatsForCompact{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1", Surface: domain.SurfaceChat}},
		conversations: stubConversationsForCompact{providerID: "compact-api-test"},
		live:          liveWith(t, engineagents.Runner{ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "compact-api-test", CurrentChatID: "chat-1"}),
	}

	err := rs.Compact(context.Background(), "chat-1")
	require.ErrorIs(t, err, problem.ErrUnavailable)
	require.NotErrorIs(t, err, ErrCompactionOffSurface)
}
