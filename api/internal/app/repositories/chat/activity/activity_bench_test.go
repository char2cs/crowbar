package activity_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// BenchmarkToolCallsInOneTurn measures one tool call (invoke + complete) deep
// inside a long turn, over file-backed stores as in production. Every command
// loads the aggregate from its last snapshot plus the events after it, so a
// turn that snapshots only at its edges pays for every earlier tool call again
// on each new one.
func BenchmarkToolCallsInOneTurn(b *testing.B) {
	for _, depth := range []int{10, 200} {
		b.Run(fmt.Sprintf("after_%d_calls", depth), func(b *testing.B) {
			repo := newBenchRepo(b)
			ctx := context.Background()
			require.NoError(b, repo.OpenTurn(ctx, activity.TurnInput{
				ChatID: chat, TurnID: "turn", ProviderID: "claude", RunnerID: "r1", SessionID: "s1", Now: t0,
			}))
			call := func(i int) {
				id := fmt.Sprintf("tool-%d", i)
				require.NoError(b, repo.InvokeTool(ctx, activity.ToolInput{ChatID: chat, ToolID: id, Name: "Read", Now: t0}))
				require.NoError(b, repo.CompleteTool(ctx, activity.ToolResultInput{ChatID: chat, ToolID: id, Name: "Read", Now: t0}))
			}
			for i := range depth {
				call(i)
			}
			b.ResetTimer()
			for i := range b.N {
				call(depth + i)
			}
		})
	}
}

func newBenchRepo(b *testing.B) activity.EventStore {
	b.Helper()
	dir := b.TempDir()
	es, err := eventsqlite.NewEventStore(filepath.Join(dir, "events.db"))
	require.NoError(b, err)
	ss, err := eventsqlite.NewSnapshotStore(filepath.Join(dir, "snapshots.db"))
	require.NoError(b, err)
	ax, err := asynx.New[domain.ChatActivity]().WithEventStore(es).WithSnapshotStore(ss).Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(filepath.Join(dir, "store.db"))
	require.NoError(b, err)
	repo, err := activity.NewEventSourced(ax, es, db, filepath.Join(dir, "content"))
	require.NoError(b, err)
	return repo
}
