package resources

import (
	"context"
	"sync"
	"time"

	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
	"github.com/char2cs/crowbar/api/internal/engine/provider/poll"
)

// Deps is what the resources read and drive.
type Deps struct {
	Workspace workspacerepo.Workspace
	Git       enginegit.Engine
	FS        enginefs.Engine
	LSP       WorkspaceReleaser
	Forge     ProviderPoller
	Now       func() time.Time
}

// Set is every resource a scope runs, rooted on one context Close cancels.
type Set struct {
	watcher    *Watcher
	hostWatch  *HostWatch
	lsp        *LSP
	forgePoll  *ForgePoll
	originSync *OriginSync
	cancel     context.CancelFunc
	closeOnce  sync.Once
}

// NewSet builds the resources over deps; signals is where their changes go.
func NewSet(ctx context.Context, signals Signaler, deps Deps) *Set {
	root, cancel := context.WithCancel(ctx)
	return &Set{
		watcher:    NewWatcher(root, signals, deps.Workspace, deps.Git, deps.FS, deps.Now),
		hostWatch:  NewHostWatch(root, signals, deps.FS),
		lsp:        NewLSP(root, deps.LSP),
		forgePoll:  NewForgePoll(root, poll.PerConnectionInterval, deps.Forge),
		originSync: NewOriginSync(root, OriginSyncInterval, deps.Workspace, deps.Git),
		cancel:     cancel,
	}
}

// ByKind names what each scope kind runs; git: and files: share one watcher.
func (s *Set) ByKind() map[stream.ScopeKind][]appstream.Resource {
	return map[stream.ScopeKind][]appstream.Resource{
		stream.KindWs:    {s.forgePoll},
		stream.KindGit:   {s.watcher, s.originSync},
		stream.KindFiles: {s.watcher},
		stream.KindLsp:   {s.lsp},
		stream.KindDoc:   {s.hostWatch},
	}
}

// Close stops every resource still running. It is idempotent.
func (s *Set) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.watcher.Close()
		s.hostWatch.Close()
		s.lsp.Close()
		s.forgePoll.Close()
		s.originSync.Close()
	})
}
