package resources

import (
	"context"
	"log/slog"
	"time"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

// watcherDispatcher fans a debounced filesystem event out to its three sinks:
// the workspace's files: and git: scopes, and the Workspace aggregate's
// SyncWorkingTreeState command (which publishes the projected row itself).
type watcherDispatcher struct {
	signals   Signaler
	workspace workspacerepo.Workspace
	now       func() time.Time
}

// newWatcherDispatcher builds a watcherDispatcher over the stream, the
// workspace repository, and a clock.
func newWatcherDispatcher(
	signals Signaler,
	workspace workspacerepo.Workspace,
	now func() time.Time,
) *watcherDispatcher {
	return &watcherDispatcher{
		signals:   signals,
		workspace: workspace,
		now:       now,
	}
}

var _ enginefs.Dispatcher = (*watcherDispatcher)(nil)

func (d *watcherDispatcher) OnFileChange(
	_ context.Context,
	evt domain.FileChangeEvent,
) {
	d.signals.Signal(stream.Scope{Kind: stream.KindFiles, Key: evt.WsID}, stream.SignalFileChange, evt)
}

func (d *watcherDispatcher) OnGitStatus(
	_ context.Context,
	wsID string,
	status gitdomain.GitStatus,
) {
	// A clean tree carries a nil Files slice; the client reads files: [], never null.
	if status.Files == nil {
		status.Files = []gitdomain.GitFile{}
	}
	d.signals.Signal(stream.Scope{Kind: stream.KindGit, Key: wsID}, stream.SignalGitStatus, status)
}

func (d *watcherDispatcher) OnSyncWorkingTreeState(
	ctx context.Context,
	input enginefs.SyncInput,
) {
	if _, err := d.workspace.ObserveWorkingTreeState(
		ctx,
		workspacerepo.SyncInput{
			ID:           input.WsID,
			Added:        input.Added,
			Deleted:      input.Deleted,
			HasConflicts: input.HasConflicts,
			HasCommits:   input.HasCommits,
		},
		d.now(),
	); err != nil {
		slog.WarnContext(ctx, "resources: sync working-tree state", "ws", input.WsID, "err", err)
	}
}
