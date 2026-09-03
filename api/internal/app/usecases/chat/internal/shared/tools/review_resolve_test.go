package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestResolveReviewThread_Resolves(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "resolve_review_thread",
		json.RawMessage(`{"threadId":"t1"}`))
	require.NoError(t, err)
	require.Equal(t, []string{"t1"}, spy.resolved)
}

func TestResolveReviewThread_RejectsAThreadOutsideTheCallersScope(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t9", WsID: "other-repo-ws"}}
	ts := reviewToolsOn(t, spy)

	_, err := ts.Call(context.Background(), "resolve_review_thread",
		json.RawMessage(`{"threadId":"t9"}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, spy.resolved, "an out-of-scope resolve must never reach the store")
}

func TestResolveReviewThread_BroadcastsSoAnOpenReviewPaneSeesIt(t *testing.T) {
	f := newReplyResolveFixture(t, "ws-a", domain.ReviewThread{ID: "t1", WsID: "ws-a"})

	_, err := f.ts.Call(context.Background(), "resolve_review_thread", json.RawMessage(`{"threadId":"t1"}`))
	require.NoError(t, err)

	require.Len(t, f.broadcast.frames, 1, "a resolve must reach connected clients")
	frame := f.broadcast.frames[0]
	require.Equal(t, "t1", frame.thread.ID)
	require.Equal(t, "P", frame.projectID)
	require.Equal(t, "R", frame.repoID)
}

func TestResolveReviewThread_RejectedCallBroadcastsNothing(t *testing.T) {
	f := newReplyResolveFixture(t, "ws-a", domain.ReviewThread{ID: "t9", WsID: "other-repo-ws"})

	_, err := f.ts.Call(context.Background(), "resolve_review_thread", json.RawMessage(`{"threadId":"t9"}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, f.broadcast.frames, "an out-of-scope resolve must not announce a thread the caller cannot see")
}
