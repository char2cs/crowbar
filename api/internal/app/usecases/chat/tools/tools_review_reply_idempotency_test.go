package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// replyStore is the thread port for the reply-dedup tests: the read half answers
// every Get with one of its threads, the write half records every reply, and err
// makes the write fail on demand.
//
// It is separate from spyThreads rather than an extension of it because the
// failure case needs a write that fails, and widening a double every other test in
// the package already depends on is how a fixture quietly changes what those tests
// mean.
type replyStore struct {
	threads []domain.ReviewThread
	replied []reviewthread.ReplyInput
	err     error
}

func (s *replyStore) Get(_ context.Context, id string) (domain.ReviewThread, error) {
	for _, t := range s.threads {
		if t.ID == id {
			return t, nil
		}
	}
	return domain.ReviewThread{}, apperrNotFound()
}

func (s *replyStore) ListByWorkspace(_ context.Context, wsID string) ([]domain.ReviewThread, error) {
	out := make([]domain.ReviewThread, 0, len(s.threads))
	for _, t := range s.threads {
		if t.WsID == wsID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *replyStore) Open(
	_ context.Context,
	in reviewthread.OpenInput,
	_ time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{ID: "new-thread", WsID: in.WsID}, nil
}

// Reply hands back an aggregate carrying the reply's own body, so a test can tell
// the fan-out payload of a real write apart from a cached one.
func (s *replyStore) Reply(
	_ context.Context,
	in reviewthread.ReplyInput,
	now time.Time,
) (domain.ReviewThread, error) {
	if s.err != nil {
		return domain.ReviewThread{}, s.err
	}
	s.replied = append(s.replied, in)
	for _, t := range s.threads {
		if t.ID != in.ID {
			continue
		}
		t.Messages = append(t.Messages, domain.ReviewMessage{
			ID: in.MessageID, Author: in.Author, IsAgent: in.IsAgent, Body: in.Body, CreatedAt: now,
		})
		return t, nil
	}
	return domain.ReviewThread{}, apperrNotFound()
}

func (s *replyStore) Resolve(_ context.Context, id string) (domain.ReviewThread, error) {
	return domain.ReviewThread{ID: id}, nil
}

// repliedIDs is the thread each recorded reply targeted, in order.
func (s *replyStore) repliedIDs() []string {
	out := make([]string, 0, len(s.replied))
	for _, in := range s.replied {
		out = append(out, in.ID)
	}
	return out
}

// replyFixture is the reply write surface with every long-lived dependency held
// separately, so a retry can be issued through a SECOND ToolSet over the same
// store, dedup map and broadcaster — which is the production shape: a ToolSet is
// built per MCP request, so the original call and the retry that follows a broken
// pipe are served by different ToolSets over one set of daemon-lived dependencies.
type replyFixture struct {
	ts        *tools.ToolSet
	store     *replyStore
	broadcast *spyThreadBroadcast
	idem      *tools.Idempotency
}

func replyOn(t *testing.T, callerWs string, threads ...domain.ReviewThread) *replyFixture {
	t.Helper()
	return newReplyIdemFixture(
		t, callerWs,
		&replyStore{threads: threads}, tools.NewIdempotency(), &spyThreadBroadcast{},
	)
}

// retryOn is the same caller arriving again on a fresh ToolSet.
func (f *replyFixture) retryOn(t *testing.T, callerWs string) *replyFixture {
	t.Helper()
	return newReplyIdemFixture(t, callerWs, f.store, f.idem, f.broadcast)
}

// withoutDedupMap is the miswired daemon: every other dependency present, no
// Idempotency at all. reply_to_review_thread still registers (see reviewTools), so
// this is reachable rather than theoretical.
func (f *replyFixture) withoutDedupMap(t *testing.T, callerWs string) *replyFixture {
	t.Helper()
	return newReplyIdemFixture(t, callerWs, f.store, nil, f.broadcast)
}

func newReplyIdemFixture(
	t *testing.T,
	callerWs string,
	store *replyStore,
	idem *tools.Idempotency,
	broadcast *spyThreadBroadcast,
) *replyFixture {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{
			ID:            "RUN",
			CurrentChatID: "CHAT",
			WorkspaceID:   callerWs,
			ProviderID:    callerProviderID,
		}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()})
	return &replyFixture{
		ts: tools.NewToolSet(tools.Deps{
			Resolver:        res,
			Threads:         store,
			ThreadWrites:    store,
			Idempotency:     idem,
			ThreadBroadcast: broadcast.fn(),
		}, "RUN", m.Mint("RUN")),
		store:     store,
		broadcast: broadcast,
		idem:      idem,
	}
}

func (f *replyFixture) reply(args string) (string, error) {
	return f.ts.Call(context.Background(), "reply_to_review_thread", json.RawMessage(args))
}

func replyArgs(threadID, body, key string) string {
	return fmt.Sprintf(`{"threadId":%q,"body":%q,"idempotencyKey":%q}`, threadID, body, key)
}

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
