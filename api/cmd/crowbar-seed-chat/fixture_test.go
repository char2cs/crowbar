package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func openAdapters(t *testing.T) *adapter.Container {
	t.Helper()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapters.Close()) })
	return adapters
}

func TestSeedChat_WritesTheRequestedTurnsAndToolCalls(t *testing.T) {
	adapters := openAdapters(t)
	ctx := context.Background()
	chatID, err := seedChat(ctx, adapters, seedOptions{
		WorkspaceID: "ws-1", ChatID: "seed-test-chat", ProviderID: "p1", Turns: 3, ToolCallsPerTurn: 2,
	})
	require.NoError(t, err)
	require.Equal(t, "seed-test-chat", chatID)

	l, err := ledgerstore.Open(adapters.ChatLedgerDB(), time.Now)
	require.NoError(t, err)
	rows, err := l.Read(ctx, chatID, 0, 0)
	require.NoError(t, err)
	kinds := map[ledger.Kind]int{}
	for _, r := range rows {
		kinds[r.Kind]++
	}
	require.Equal(t, 3, kinds[ledger.KindUserMessage])
	require.Equal(t, 3, kinds[ledger.KindTurnEnded])
	require.Equal(t, 3*2*2, kinds[ledger.KindToolCall], "each call is invoked and completed")
	require.Equal(t, 3, kinds[ledger.KindAssistantText], "one reply per turn")
}

func TestSeedChat_IsBornOnARegisteredProvider(t *testing.T) {
	adapters := openAdapters(t)
	ctx := context.Background()
	chatID, err := seedChat(ctx, adapters, seedOptions{WorkspaceID: "ws-1", Turns: 1, ToolCallsPerTurn: 1})
	require.NoError(t, err)

	axAgentChat, err := buildAsynx[domain.Chat](adapters.AgentChatES(), adapters.AgentChatSS())
	require.NoError(t, err)
	chatStore, err := agentchat.NewEventSourced(axAgentChat, adapters.AgentChatES(), adapters.AgentChatReadDB(), nil)
	require.NoError(t, err)
	chat, err := chatStore.GetChat(ctx, chatID)
	require.NoError(t, err)
	engine := engineagents.New()
	defer engine.Close()
	_, err = engine.Get(ctx, adapters.Home().Root(), chat.ProviderID)
	require.NoError(t, err, "a chat with no provider is refused by provider-scoped reads such as the slash catalog")
}

func TestSeedChat_GeneratesAChatIDWhenNoneGiven(t *testing.T) {
	chatID, err := seedChat(context.Background(), openAdapters(t), seedOptions{
		WorkspaceID: "ws-1", ProviderID: "p1", Turns: 1, ToolCallsPerTurn: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
}
