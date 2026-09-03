package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The trigger this key exists for: the relay's writeLine fails on a broken pipe
// AFTER the daemon committed the reply, so the client never sees a result and
// retries — on a NEW ToolSet, which is why the dedup map is a daemon-lived
// dependency rather than ToolSet state.
func TestReplyToReviewThread_IdempotencyKeyCollapsesARetry(t *testing.T) {
	f := replyOn(t, "ws-a", domain.ReviewThread{ID: "t1", WsID: "ws-a"})
	args := replyArgs("t1", "Bounded the loop.", "answered-t1")

	first, err := f.reply(args)
	require.NoError(t, err)

	second, err := f.retryOn(t, "ws-a").reply(args)
	require.NoError(t, err)

	require.Equal(t, first, second, "a retry must be answered exactly as the original was")
	require.Equal(t, []string{"t1"}, f.store.repliedIDs(),
		"the retry must not append a second copy of the reply")
	require.Len(t, f.broadcast.frames, 1,
		"a retry that wrote nothing must not tell every client to re-render")
}

// Different keys are different writes. Without this the collapse test above would
// still pass against an implementation that deduped on the thread id alone.
func TestReplyToReviewThread_DifferentKeysBothLand(t *testing.T) {
	f := replyOn(t, "ws-a", domain.ReviewThread{ID: "t1", WsID: "ws-a"})

	_, err := f.reply(replyArgs("t1", "Bounded the loop.", "answered-t1"))
	require.NoError(t, err)
	_, err = f.retryOn(t, "ws-a").reply(replyArgs("t1", "And named the constant.", "named-const"))
	require.NoError(t, err)

	require.Equal(t, []string{"t1", "t1"}, f.store.repliedIDs())
	require.Len(t, f.broadcast.frames, 2)
}

// No key means no dedup, which is what every call made before this task did and
// what an agent that never passes one still gets.
func TestReplyToReviewThread_WithoutAKeyEveryCallWrites(t *testing.T) {
	f := replyOn(t, "ws-a", domain.ReviewThread{ID: "t1", WsID: "ws-a"})
	args := `{"threadId":"t1","body":"Bounded the loop."}`

	_, err := f.reply(args)
	require.NoError(t, err)
	_, err = f.retryOn(t, "ws-a").reply(args)
	require.NoError(t, err)

	require.Len(t, f.store.replied, 2)
	require.Len(t, f.broadcast.frames, 2)
}

// The key is scoped by WORKSPACE, so two agents reviewing two branches cannot
// collide on the same obvious key and have the second answer silently swallowed.
// ws-a and ws-a1 are different callers that can BOTH see the thread on ws-a1, so
// the only thing separating their keys is the scoping.
func TestReplyToReviewThread_SameKeyInTwoWorkspacesDoesNotCollide(t *testing.T) {
	f := replyOn(t, "ws-a", domain.ReviewThread{ID: "t2", WsID: "ws-a1"})
	args := replyArgs("t2", "Acknowledged.", "acknowledged")

	_, err := f.reply(args)
	require.NoError(t, err)
	_, err = f.retryOn(t, "ws-a1").reply(args)
	require.NoError(t, err)

	require.Equal(t, []string{"t2", "t2"}, f.store.repliedIDs(),
		"a key from another workspace must not swallow this agent's reply")
	require.Len(t, f.broadcast.frames, 2)
}

// A retry that reuses a key against a DIFFERENT thread wrote nothing, so it must
// be told which thread the reply is actually on. Echoing its own argument would
// name a thread that never received the reply — the same trap post_review_comment
// avoids by reporting the stored anchor.
func TestReplyToReviewThread_ARetryReportsTheThreadItLandedOn(t *testing.T) {
	f := replyOn(t, "ws-a",
		domain.ReviewThread{ID: "t1", WsID: "ws-a"},
		domain.ReviewThread{ID: "t7", WsID: "ws-a"})

	_, err := f.reply(replyArgs("t1", "Bounded the loop.", "k"))
	require.NoError(t, err)

	out, err := f.retryOn(t, "ws-a").reply(replyArgs("t7", "Bounded the loop.", "k"))
	require.NoError(t, err)

	require.Contains(t, out, "t1", "the reply landed on t1, so t1 is what must be reported")
	require.NotContains(t, out, "t7")
	require.Equal(t, []string{"t1"}, f.store.repliedIDs())
}

// A failed write must not be remembered as done, or the retry the key exists for
// would be answered with success having stored nothing.
func TestReplyToReviewThread_AFailedWriteIsNotRemembered(t *testing.T) {
	f := replyOn(t, "ws-a", domain.ReviewThread{ID: "t1", WsID: "ws-a"})
	args := replyArgs("t1", "Bounded the loop.", "k")
	f.store.err = errNotFoundForTest

	_, err := f.reply(args)
	require.Error(t, err)
	require.Empty(t, f.broadcast.frames, "nothing was stored, so nothing may be announced")

	f.store.err = nil
	_, err = f.retryOn(t, "ws-a").reply(args)
	require.NoError(t, err)
	require.Equal(t, []string{"t1"}, f.store.repliedIDs())
	require.Len(t, f.broadcast.frames, 1)
}

// The scope check runs BEFORE the dedup, on every call. A rejected reply must also
// leave no trace in the map, or the key it carried would answer the caller's next
// legitimate reply with a write that never happened.
func TestReplyToReviewThread_ARejectedReplyDoesNotClaimTheKey(t *testing.T) {
	f := replyOn(t, "ws-a",
		domain.ReviewThread{ID: "t9", WsID: "ws-b"},
		domain.ReviewThread{ID: "t1", WsID: "ws-a"})

	_, err := f.reply(replyArgs("t9", "should not land", "k"))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, f.store.replied, "an out-of-scope reply must never reach the store")

	out, err := f.retryOn(t, "ws-a").reply(replyArgs("t1", "Bounded the loop.", "k"))
	require.NoError(t, err)
	require.Contains(t, out, "t1")
	require.Equal(t, []string{"t1"}, f.store.repliedIDs())
}

// A key the daemon cannot honour is refused rather than quietly ignored: a caller
// that supplied one is retrying, and writing it unguarded is precisely the
// duplicate the key was passed to prevent. An UNKEYED reply on the same miswired
// daemon still works — it never asked for dedup.
func TestReplyToReviewThread_AKeyedReplyIsRefusedWithoutADedupMap(t *testing.T) {
	f := replyOn(t, "ws-a", domain.ReviewThread{ID: "t1", WsID: "ws-a"})
	broken := f.withoutDedupMap(t, "ws-a")

	_, err := broken.reply(replyArgs("t1", "Bounded the loop.", "k"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "idempotencyKey")
	require.Empty(t, f.store.replied, "a key that cannot be honoured must not write unguarded")

	_, err = broken.reply(`{"threadId":"t1","body":"Bounded the loop."}`)
	require.NoError(t, err)
	require.Equal(t, []string{"t1"}, f.store.repliedIDs())
}

// The dedup map is shared with post_review_comment, so the two verbs must not
// share a key NAMESPACE: a model naming a finding once when it posts and again
// when it replies is naming the same finding, not the same write.
func TestReplyToReviewThread_DoesNotShareAKeyspaceWithPostReviewComment(t *testing.T) {
	f := replyOn(t, "ws-a", domain.ReviewThread{ID: "t1", WsID: "ws-a"})

	_, err := f.reply(replyArgs("t1", "Bounded the loop.", "nil-deref-in-auth"))
	require.NoError(t, err)

	post := newPostFixture(
		t, "ws-a", authHunk(),
		&stubThreadWriter{}, f.idem, &spyThreadBroadcast{},
	)
	out, err := post.post(
		`{"filePath":"src/auth.go","startLine":42,"endLine":42,"side":"right",` +
			`"body":"leak","idempotencyKey":"nil-deref-in-auth"}`,
	)
	require.NoError(t, err)
	require.Len(t, post.writer.opens, 1,
		"a reply's key must not make the post that shares its name look like a retry")
	require.Contains(t, out, "thread-1")
}

func TestReplyToReviewThread_AppendsAsAgent(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)

	out, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t1","body":"Bounded the loop."}`))
	require.NoError(t, err)
	require.Contains(t, out, "t1")
	require.Equal(t, []string{"t1"}, spy.repliedIDs())
}

// A blank body is rejected before the thread is ever looked up, mirroring
// post_review_comment's own blank-body case: an empty reply is not a finding a
// user should ever see land in their review pane.
func TestReplyToReviewThread_RejectsABlankBody(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t1","body":"   "}`))
	require.Error(t, err)
	require.Empty(t, spy.replied, "a blank reply must never reach the store")
}

// The scope hole to close: a thread id names a thread in SOME workspace, so the
// id itself is not an authorization. ws-b is a sibling the caller cannot see.
func TestReplyToReviewThread_RejectsAThreadOutsideTheCallersScope(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t9", WsID: "ws-b"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t9","body":"should not land"}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, spy.replied, "an out-of-scope reply must never reach the store")
}

// An ANCESTOR is out of scope too — visibility is downward only.
func TestReplyToReviewThread_RejectsAThreadOnAnAncestorWorkspace(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t0", WsID: "repo-default"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t0","body":"upward is forbidden"}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, spy.replied)
}

// A DESCENDANT is in scope.
func TestReplyToReviewThread_AllowsAThreadOnADescendantWorkspace(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t2", WsID: "ws-a1"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t2","body":"ok"}`))
	require.NoError(t, err)
	require.Equal(t, []string{"t2"}, spy.repliedIDs())
}

// TestReplyToReviewThread_BroadcastsSoAnOpenReviewPaneSeesIt models
// TestPostReviewComment_BroadcastsSoAnOpenReviewPaneSeesIt: the review-thread
// store does not fan out on its own, and an agent's reply bypasses the HTTP
// handler that normally pushes a frame, so without an explicit broadcast the
// reply is stored and invisible until the user remounts the pane.
func TestReplyToReviewThread_BroadcastsSoAnOpenReviewPaneSeesIt(t *testing.T) {
	f := newReplyResolveFixture(t, "ws-a", domain.ReviewThread{ID: "t1", WsID: "ws-a"})

	_, err := f.ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t1","body":"Bounded the loop."}`))
	require.NoError(t, err)

	require.Len(t, f.broadcast.frames, 1, "a stored reply must reach connected clients")
	frame := f.broadcast.frames[0]
	require.Equal(t, "t1", frame.thread.ID)
	require.Equal(t, "P", frame.projectID)
	require.Equal(t, "R", frame.repoID)
}

// TestReplyToReviewThread_BroadcastsUnderTheThreadsOwnWorkspace is the case
// TestPostReviewComment's fixtures cannot exercise: post_review_comment only ever
// writes to the caller's OWN workspace, so its ids and the written thread's ids
// always coincide. reply/resolve can write to any workspace CanSee allows, and a
// caller at "home" sees other-repo-ws — a DIFFERENT repo from its own (home has
// no repo at all). Broadcasting under the caller's own (empty) repo would
// deliver the frame to nobody's /threads stream.
func TestReplyToReviewThread_BroadcastsUnderTheThreadsOwnWorkspace(t *testing.T) {
	f := newReplyResolveFixture(t, "home", domain.ReviewThread{ID: "t9", WsID: "other-repo-ws"})

	_, err := f.ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t9","body":"ok"}`))
	require.NoError(t, err)

	require.Len(t, f.broadcast.frames, 1)
	require.Equal(t, "R2", f.broadcast.frames[0].repoID,
		"the frame must carry the THREAD's own repo, not the home caller's (empty) one")
}

func TestReplyToReviewThread_RejectedCallBroadcastsNothing(t *testing.T) {
	f := newReplyResolveFixture(t, "ws-a", domain.ReviewThread{ID: "t9", WsID: "ws-b"})

	_, err := f.ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t9","body":"should not land"}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, f.broadcast.frames, "an out-of-scope reply must not announce a thread the caller cannot see")
}

// TestReplyToReviewThread_CarriesTheCallersProviderAndChat is the same property
// for the reply path, which is where two agents in one thread actually become
// indistinguishable without it.
func TestReplyToReviewThread_CarriesTheCallersProviderAndChat(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := attributedReviewToolsOn(t, spy, "codex", "chat-9")

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t1","body":"Bounded the loop."}`))
	require.NoError(t, err)

	require.Len(t, spy.replied, 1)
	require.Equal(t, "codex", spy.replied[0].ProviderID)
	require.Equal(t, "chat-9", spy.replied[0].ChatID)
	require.True(t, spy.replied[0].IsAgent)
	require.NotEmpty(t, spy.replied[0].MessageID)
}

// TestReplyToReviewThread_AttributesTheWriterNotTheThreadsWorkspace pins the case
// a caller can reach a DESCENDANT's thread: the attribution describes who wrote
// the reply, which is the caller, and is unaffected by where the thread lives.
func TestReplyToReviewThread_AttributesTheWriterNotTheThreadsWorkspace(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t2", WsID: "ws-a1"}}
	ts := attributedReviewToolsOn(t, spy, "codex", "chat-9")

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(`{"threadId":"t2","body":"ok"}`))
	require.NoError(t, err)

	require.Len(t, spy.replied, 1)
	require.Equal(t, "codex", spy.replied[0].ProviderID)
	require.Equal(t, "chat-9", spy.replied[0].ChatID)
}

func TestReplyToReviewThread_RefusesAnOverlongBodyAndNamesTheLimit(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)
	over := tools.MaxWrittenBodyCharsForTest + 1

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(fmt.Sprintf(`{"threadId":"t1","body":%q}`, longBody(over))))

	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprint(over))
	require.Contains(t, err.Error(), fmt.Sprint(tools.MaxWrittenBodyCharsForTest))
	require.Empty(t, spy.replied, "a refused body must never reach the store")
}
