package tools_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestReviewTools_OnlyReadTheCallersOwnWorkspace is the security property: both
// tools take no workspace-like argument at all, so the ONLY wsID either can
// ever query is the one the Resolver computed for the caller.
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

// A runner with no provider recorded still writes — attribution is decoration,
// not a precondition — and the message simply carries none, which is exactly the
// shape every pre-attribution message already has. The UI's fallback path is
// therefore reachable from a live write, not only from historical data.
func TestReviewWrites_WithoutAProviderCarryNoAttribution(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t1","body":"still lands"}`))
	require.NoError(t, err)

	require.Len(t, spy.replied, 1)
	require.Empty(t, spy.replied[0].ProviderID)
	// The chat is still recorded: reviewToolsOn's runner has a current chat even
	// though it names no provider, and the two ids are independent.
	require.Equal(t, "CHAT", spy.replied[0].ChatID)
}

// TestReviewWriteTools_DoNotAcceptAttributionAsAnArgument is the forgery guard's
// first half: attribution a model can type is attribution a model can fake, and
// a finding filed under another agent's name or another chat's id is worse than
// an anonymous one — so neither id may appear in a WRITE tool's schema.
//
// It runs over the full-surface fixture and pins how many write tools it
// actually inspected, because "iterated nothing" and "found nothing wrong" are
// the same green.
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

// TestReviewWriteTools_ForgedAttributionInArgumentsNeverReachesTheStore is the
// half the schema check cannot cover, and the half that actually matters.
//
// A schema is ADVERTISEMENT: it is a hand-written JSON literal, entirely
// separate from the arg struct, and decode is a bare json.Unmarshal — so
// `additionalProperties:false` is never enforced against anything. A model can
// put whatever it likes in the arguments object. The property that has to hold
// is therefore about the STORED message, not about the schema text: whatever a
// caller types, the provider, chat and author written to the thread are the ones
// the Resolver computed from the runner.
//
// Proven necessary by mutation: adding ProviderID/ChatID to postReviewCommentArgs
// and letting a supplied value win in openInputFor stored ProviderID="IMPOSTOR"
// with the whole ./internal/... suite still green.
func TestReviewWriteTools_ForgedAttributionInArgumentsNeverReachesTheStore(t *testing.T) {
	// Every attribution field the store records, spelled the way a schema would
	// spell it and the way Go's JSON decoder would match it case-insensitively.
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

	// resolve_review_thread writes no message and therefore carries no
	// attribution to forge. It is here so the set of write tools this file
	// exercises is the same set reviewWriteToolNames names — and so a future
	// resolve that DID start writing a message would arrive without a guard and
	// be noticed.
	t.Run("resolve_review_thread stores no attribution to forge", func(t *testing.T) {
		spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
		ts := attributedReviewToolsOn(t, spy, "codex", "chat-9")

		_, err := ts.Call(context.Background(), "resolve_review_thread",
			json.RawMessage(`{"threadId":"t1",`+forged+`}`))
		require.NoError(t, err)
		require.Empty(t, spy.replied, "resolving must not append a message")
	})
}
