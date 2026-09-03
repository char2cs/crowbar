package activity

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func DispatchForTest(
	ctx context.Context,
	send func(context.Context, asynxModels.Command[domain.ChatActivity]) (asynxModels.Event[domain.ChatActivity], error),
	cmd asynxModels.Command[domain.ChatActivity],
) error {
	r := &eventSourced{}
	return r.dispatch(ctx, send, cmd)
}

const MaxOCCAttempts = maxOCCAttempts

// SetSnapshotIntervalForTest forces repo to snapshot once every n dispatched
// commands, on top of whatever each command's own ShouldSnapshot() already
// requests. n<=0 restores the production default (defer entirely to each
// command). This cannot change what a test observes: asynx's snapshot+delta
// path is required to reconstruct identical state to a full cold replay
// (asynx's own tests guard that), so this only bounds how many events a
// Load() has to walk through, never the resulting state. It exists because a
// handful of tests here drive hundreds of non-snapshotting commands
// (InvokeTool, OpenChoice, ...) at one aggregate with nothing to snapshot
// interleaved, which otherwise pays full cold replay on every single one.
func SetSnapshotIntervalForTest(repo EventStore, n int) {
	es, ok := repo.(*eventSourced)
	if !ok {
		return
	}
	es.snapshotInterval.Store(int64(n))
}
