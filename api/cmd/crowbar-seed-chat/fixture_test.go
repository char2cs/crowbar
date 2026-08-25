package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSeedChat_WritesTheRequestedTurnsAndToolCalls(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	ctx := context.Background()
	chatID, err := seedChat(ctx, adapters, seedOptions{
		WorkspaceID:      "ws-1",
		ChatID:           "seed-test-chat",
		Turns:            3,
		ToolCallsPerTurn: 2,
	})
	require.NoError(t, err)
	require.Equal(t, "seed-test-chat", chatID)

	axAgentActivity, err := buildAsynx[domain.ChatActivity](adapters.AgentActivityES(), adapters.AgentActivitySS())
	require.NoError(t, err)
	activityStore, err := agentactivity.NewEventSourced(
		axAgentActivity, adapters.AgentActivityES(), adapters.AgentActivityReadDB(),
		adapters.CrowbarHome()+"/state/content",
	)
	require.NoError(t, err)

	turnCount, err := activityStore.CountTurns(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, int64(6), turnCount, "3 user turns + 3 assistant turns")

	calls, err := activityStore.ToolCalls(ctx, chatID, 0, 100)
	require.NoError(t, err)
	require.Len(t, calls, 6, "3 assistant turns * 2 tool calls each")
	for _, c := range calls {
		require.Equal(t, domain.ToolStatusOK, c.Status)
	}
}

func TestSeedChat_GeneratesAChatIDWhenNoneGiven(t *testing.T) {
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	chatID, err := seedChat(context.Background(), adapters, seedOptions{
		WorkspaceID: "ws-1", Turns: 1, ToolCallsPerTurn: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
}
