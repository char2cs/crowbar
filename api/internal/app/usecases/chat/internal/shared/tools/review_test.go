package tools_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestReviewTools_OnlyReadTheCallersOwnWorkspace(t *testing.T) {
	threadStub := &stubThreadReader{}
	reviewStub := &stubReviewReader{}
	ts, _ := reviewToolsetOn(t, threadStub, reviewStub)

	_, err := ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ws-a", threadStub.lastWsID)

	_, err = ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ws-a", reviewStub.lastWsID)

	forbidden := []string{"wsId", "wsID", "workspaceId", "workspace_id"}
	for _, tool := range ts.Tools() {
		if tool.Name != "list_review_threads" && tool.Name != "get_review_scope" {
			continue
		}
		for _, f := range forbidden {
			require.NotContains(t, string(tool.InputSchema), f,
				"tool %s exposes %s; scope must never be an argument", tool.Name, f)
		}
	}
}

func TestReviewWrites_WithoutAProviderCarryNoAttribution(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t1","body":"still lands"}`))
	require.NoError(t, err)

	require.Len(t, spy.replied, 1)
	require.Empty(t, spy.replied[0].ProviderID)
	require.Equal(t, "CHAT", spy.replied[0].ChatID)
}

func TestReviewWriteTools_DoNotAcceptAttributionAsAnArgument(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	forbidden := []string{"providerId", "provider_id", "chatId", "chat_id", "author"}

	inspected := 0
	for _, tool := range ts.Tools() {
		if !slices.Contains(reviewWriteToolNames, tool.Name) {
			continue
		}
		inspected++
		for _, bad := range forbidden {
			require.NotContains(t, string(tool.InputSchema), bad,
				"tool %s exposes %s; attribution must come from the caller, never an argument",
				tool.Name, bad)
		}
	}
	require.Equal(t, len(reviewWriteToolNames), inspected,
		"every write tool must have been inspected; a fixture that stopped registering one "+
			"would otherwise leave this guard silently checking fewer tools")
}

func TestReviewWriteTools_ForgedAttributionInArgumentsNeverReachesTheStore(t *testing.T) {
	const forged = `"providerId":"IMPOSTOR","ProviderID":"IMPOSTOR",` +
		`"chatId":"IMPOSTOR-CHAT","ChatID":"IMPOSTOR-CHAT",` +
		`"author":"mateo","Author":"mateo","isAgent":false,"IsAgent":false`

	t.Run("post_review_comment", func(t *testing.T) {
		f := postOn(t, "ws-a", authHunk())

		_, err := f.post(`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right",` +
			`"body":"This leaks the token.",` + forged + `}`)
		require.NoError(t, err)

		require.Len(t, f.writer.opens, 1)
		got := f.writer.opens[0]
		require.Equal(t, callerProviderID, got.ProviderID,
			"the caller's provider must win over one supplied in the arguments")
		require.Equal(t, "CHAT", got.ChatID,
			"the caller's current chat must win over one supplied in the arguments")
		require.Equal(t, callerProviderID, got.Author,
			"the author is derived from the caller, never taken from the arguments")
		require.True(t, got.IsAgent,
			"an agent write must stay marked as one; a model must not be able to pose as a human")
	})

	t.Run("reply_to_review_thread", func(t *testing.T) {
		spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
		ts := attributedReviewToolsOn(t, spy, "codex", "chat-9")

		_, err := ts.Call(context.Background(), "reply_to_review_thread",
			json.RawMessage(`{"threadId":"t1","body":"Bounded the loop.",`+forged+`}`))
		require.NoError(t, err)

		require.Len(t, spy.replied, 1)
		require.Equal(t, "codex", spy.replied[0].ProviderID)
		require.Equal(t, "chat-9", spy.replied[0].ChatID)
		require.Equal(t, "codex", spy.replied[0].Author)
		require.True(t, spy.replied[0].IsAgent)
	})

	t.Run("resolve_review_thread stores no attribution to forge", func(t *testing.T) {
		spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
		ts := attributedReviewToolsOn(t, spy, "codex", "chat-9")

		_, err := ts.Call(context.Background(), "resolve_review_thread",
			json.RawMessage(`{"threadId":"t1",`+forged+`}`))
		require.NoError(t, err)
		require.Empty(t, spy.replied, "resolving must not append a message")
	})
}
