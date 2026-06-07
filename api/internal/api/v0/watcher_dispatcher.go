package v0

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

// watcherDispatcher fans a single debounced filesystem event out to the three
// realtime sinks defined in 03 §5: the Files topic (BroadcastFile), the Git
// topic (BroadcastGit), and the Workspace aggregate's SyncWorkingTreeState
// command (which itself re-broadcasts the projected Workspace row).
type watcherDispatcher struct {
	hub       *hub.Hub
	workspace workspacerepo.Workspace
	now       func() time.Time
}

// newWatcherDispatcher builds a watcherDispatcher over the hub, the workspace
// repository, and a clock.
func newWatcherDispatcher(
	h *hub.Hub,
	workspace workspacerepo.Workspace,
	now func() time.Time,
) *watcherDispatcher {
	return &watcherDispatcher{
		hub:       h,
		workspace: workspace,
		now:       now,
	}
}

var _ enginefs.Dispatcher = (*watcherDispatcher)(nil)

func (d *watcherDispatcher) OnFileChange(
	_ context.Context,
	evt domain.FileChangeEvent,
) {
	d.hub.BroadcastFile(evt)
}

func (d *watcherDispatcher) OnGitStatus(
	_ context.Context,
	wsID string,
	status gitdomain.GitStatus,
) {
	d.hub.BroadcastGit(wsID, status)
}

func (d *watcherDispatcher) OnSyncWorkingTreeState(
	ctx context.Context,
	input enginefs.SyncInput,
) {
	_, _ = d.workspace.SyncWorkingTreeState(
		ctx,
		workspacerepo.SyncInput{
			ID:           input.WsID,
			Added:        input.Added,
			Deleted:      input.Deleted,
			HasConflicts: input.HasConflicts,
			HasCommits:   input.HasCommits,
		},
		d.now(),
	)
}
