package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// listSignalReader tells the test each time the tool has listed, which happens only
// after it subscribed — so a notification sent after the signal cannot be lost.
type listSignalReader struct {
	*stubThreadReader
	listed chan struct{}
}

func (r *listSignalReader) ListByWorkspace(ctx context.Context, wsID string) ([]domain.ReviewThread, error) {
	out, err := r.stubThreadReader.ListByWorkspace(ctx, wsID)
	select {
	case r.listed <- struct{}{}:
	default:
	}
	return out, err
}

func waitThread(id, ws string, status domain.ReviewThreadStatus, lastIsAgent bool) domain.ReviewThread {
	return domain.ReviewThread{
		ID: id, WsID: ws, FilePath: "a.go", StartLine: 1, EndLine: 1,
		Side: domain.ReviewSideRight, Status: status,
		Messages: []domain.ReviewMessage{
			{ID: id + "-m1", Author: "mateo", Body: "first " + id},
			{ID: id + "-m2", Author: "who", IsAgent: lastIsAgent, Body: "last " + id},
		},
	}
}

type waitFixture struct {
	ts      *tools.ToolSet
	deps    tools.Deps
	changes *reviewthread.Changes
	reader  *listSignalReader
}

func waitOn(t *testing.T, threads ...domain.ReviewThread) *waitFixture {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	f := &waitFixture{
		changes: reviewthread.NewChanges(),
		reader: &listSignalReader{
			stubThreadReader: &stubThreadReader{list: threads},
			listed:           make(chan struct{}, 8),
		},
	}
	f.deps = tools.Deps{Resolver: res, Threads: f.reader, ThreadChanges: f.changes}
	f.ts = tools.NewToolSet(f.deps, "RUN", m.Mint("RUN"))
	return f
}

func (f *waitFixture) call(ctx context.Context, args string) (string, error) {
	return f.ts.Call(ctx, "wait_for_review_comments", json.RawMessage(args))
}

type waitResult struct {
	out string
	err error
}

func (f *waitFixture) callAsync(ctx context.Context, args string) <-chan waitResult {
	done := make(chan waitResult, 1)
	go func() {
		out, err := f.call(ctx, args)
		done <- waitResult{out, err}
	}()
	return done
}

func TestWaitForReviewComments_ReturnsImmediatelyForAHumanLastUnresolvedThread(t *testing.T) {
	f := waitOn(t, waitThread("t1", "ws-a", domain.ReviewThreadStatusOpen, false))

	out, err := f.call(context.Background(), `{}`)

	require.NoError(t, err)
	require.Contains(t, out, "1 review thread needs your attention")
	require.Contains(t, out, "first t1")
	require.Contains(t, out, "last t1")
}

func TestWaitForReviewComments_IgnoresResolvedAgentLastAndForeignThreads(t *testing.T) {
	f := waitOn(t,
		waitThread("resolved", "ws-a", domain.ReviewThreadStatusResolved, false),
		waitThread("answered", "ws-a", domain.ReviewThreadStatusOpen, true),
		waitThread("foreign", "ws-b", domain.ReviewThreadStatusOpen, false),
	)

	got, err := tools.WaitForReviewCommentsForTest(context.Background(), f.deps, "ws-a", time.Millisecond)

	require.NoError(t, err)
	require.Contains(t, got, "No new review comments")
	require.NotContains(t, got, "resolved")
	require.NotContains(t, got, "foreign")
}

func TestWaitForReviewComments_BlocksThenWakesOnANotification(t *testing.T) {
	f := waitOn(t)
	done := f.callAsync(context.Background(), `{"timeoutSeconds":100}`)
	<-f.reader.listed

	f.changes.Notify(waitThread("t1", "ws-a", domain.ReviewThreadStatusOpen, false))

	res := <-done
	require.NoError(t, res.err)
	require.Contains(t, res.out, "last t1")
}

// ListByWorkspace reads a read model projected independently of the notification,
// so it can still show the old state when the wake arrives.
func TestWaitForReviewComments_TheNotificationOverridesAStaleReadModel(t *testing.T) {
	stale := waitThread("t1", "ws-a", domain.ReviewThreadStatusOpen, true)
	f := waitOn(t, stale)
	done := f.callAsync(context.Background(), `{"timeoutSeconds":100}`)
	<-f.reader.listed

	f.changes.Notify(waitThread("t1", "ws-a", domain.ReviewThreadStatusOpen, false))

	res := <-done
	require.NoError(t, res.err)
	require.Contains(t, res.out, "1 review thread needs your attention")
	require.Equal(t, 1, strings.Count(res.out, "last t1"), "the thread renders once, not stale and fresh")
}

func TestWaitForReviewComments_AWakeForAnAnsweredThreadKeepsWaiting(t *testing.T) {
	f := waitOn(t)
	done := f.callAsync(context.Background(), `{"timeoutSeconds":100}`)
	<-f.reader.listed

	f.changes.Notify(waitThread("t1", "ws-a", domain.ReviewThreadStatusOpen, true))
	f.changes.Notify(waitThread("t2", "ws-a", domain.ReviewThreadStatusOpen, false))

	res := <-done
	require.NoError(t, res.err)
	require.Contains(t, res.out, "last t2")
	require.NotContains(t, res.out, "last t1")
}

func TestWaitForReviewComments_OtherWorkspacesNeverWakeIt(t *testing.T) {
	f := waitOn(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := f.callAsync(ctx, `{"timeoutSeconds":100}`)
	<-f.reader.listed

	f.changes.Notify(waitThread("t1", "ws-b", domain.ReviewThreadStatusOpen, false))
	cancel()

	res := <-done
	require.ErrorIs(t, res.err, context.Canceled)
}

func TestWaitForReviewComments_TimingOutSaysToCallAgain(t *testing.T) {
	f := waitOn(t)

	out, err := tools.WaitForReviewCommentsForTest(context.Background(), f.deps, "ws-a", time.Millisecond)

	require.NoError(t, err)
	require.Contains(t, out, "No new review comments")
	require.Contains(t, out, "wait_for_review_comments")
}

func TestWaitForReviewComments_CancellationReturnsTheContextError(t *testing.T) {
	f := waitOn(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := f.callAsync(ctx, `{}`)
	<-f.reader.listed

	cancel()

	require.ErrorIs(t, (<-done).err, context.Canceled)
}

func TestWaitForReviewComments_ClampsTheTimeout(t *testing.T) {
	require.Equal(t, 50*time.Second, tools.ReviewWaitTimeoutForTest(0))
	require.Equal(t, time.Second, tools.ReviewWaitTimeoutForTest(-5))
	require.Equal(t, time.Second, tools.ReviewWaitTimeoutForTest(1))
	require.Equal(t, 30*time.Second, tools.ReviewWaitTimeoutForTest(30))
	require.Equal(t, 100*time.Second, tools.ReviewWaitTimeoutForTest(100))
	require.Equal(t, 100*time.Second, tools.ReviewWaitTimeoutForTest(100000))
}

func TestWaitForReviewComments_IsWithdrawnWithoutItsPorts(t *testing.T) {
	f := waitOn(t)
	for name, deps := range map[string]tools.Deps{
		"no change source": {Resolver: f.deps.Resolver, Threads: f.reader},
		"no thread reader": {Resolver: f.deps.Resolver, ThreadChanges: f.changes},
	} {
		for _, tool := range tools.NewToolSet(deps, "RUN", "x").Tools() {
			require.NotEqual(t, "wait_for_review_comments", tool.Name, name)
		}
	}
	var advertised []string
	for _, tool := range f.ts.Tools() {
		advertised = append(advertised, tool.Name)
	}
	require.Contains(t, advertised, "wait_for_review_comments")
}

func TestWaitForReviewComments_TakesNoWorkspaceArgument(t *testing.T) {
	f := waitOn(t)
	for _, tool := range f.ts.Tools() {
		if tool.Name != "wait_for_review_comments" {
			continue
		}
		for _, bad := range []string{"wsId", "wsID", "workspaceId", "workspace_id"} {
			require.NotContains(t, string(tool.InputSchema), bad)
		}
	}
}
