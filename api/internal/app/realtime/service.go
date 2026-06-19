package realtime

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// Service is the app-layer owner of the lazy realtime resource lifecycles
// (03 §6). It holds the refcounted WatcherManager spanning the Files∪Git topics
// and the independent LSPManager, rooting every per-workspace goroutine on a
// cancelable context derived from the ctx passed to New. The API layer wires its
// broadcaster OnSubscribe/OnUnsubscribe hooks to the Acquire/Release methods so
// the watcher starts on the first (Files∪Git) subscriber and stops on the last,
// and the LSP host starts on the first LSP subscriber and stops on the last;
// Close tears every live resource down on graceful shutdown.
//
// The dispatcher this service drives issues the SyncWorkingTreeState aggregate
// command and the hub.BroadcastFile/BroadcastGit fan-out, which correctly live
// in the app layer (matching RegisterHubProjections): producers and hub
// broadcasts originate in app, never in the delivery layer.
type Service struct {
	watchers     *WatcherManager
	lsps         *LSPManager
	providerPoll *ProviderPollManager
	cancel       context.CancelFunc
	closeOnce    sync.Once
}

// New builds the realtime Service from the hub, the workspace repository, the
// git and fs engines, the LSP lifecycle, the provider-poll usecase, the
// per-connection poll interval, and a clock. It derives a cancelable child of
// ctx so Close can stop every watcher, LSP host, and provider poll on graceful
// shutdown; New must not block, so the goroutines are started lazily on Acquire.
func New(
	ctx context.Context,
	h *hub.Hub,
	workspace workspacerepo.Workspace,
	gitEngine enginegit.Engine,
	fsEngine enginefs.Engine,
	lspLifecycle LSPLifecycle,
	providerPoll ProviderPoller,
	perConnPollInterval time.Duration,
	now func() time.Time,
) *Service {
	root, cancel := context.WithCancel(ctx)
	return &Service{
		watchers:     NewWatcherManager(root, h, workspace, gitEngine, fsEngine, now),
		lsps:         NewLSPManager(root, lspLifecycle),
		providerPoll: NewProviderPollManager(root, perConnPollInterval, providerPoll),
		cancel:       cancel,
	}
}

// AcquireWatcher records one Files∪Git subscriber for wsID, starting the file
// watcher on the 0→1 transition.
func (s *Service) AcquireWatcher(
	wsID string,
) {
	s.watchers.Acquire(wsID)
}

// ReleaseWatcher drops one Files∪Git subscriber for wsID, stopping the file
// watcher on the 1→0 transition.
func (s *Service) ReleaseWatcher(
	wsID string,
) {
	s.watchers.Release(wsID)
}

// AcquireLSP records one LSP subscriber for wsID, running the LSP lifecycle
// Ensure on the 0→1 transition.
func (s *Service) AcquireLSP(
	wsID string,
) {
	s.lsps.Acquire(wsID)
}

// ReleaseLSP drops one LSP subscriber for wsID, running the LSP lifecycle
// Shutdown on the 1→0 transition.
func (s *Service) ReleaseLSP(
	wsID string,
) {
	s.lsps.Release(wsID)
}

// AcquireProviderPoll records one single-workspace WS subscriber for wsID,
// starting the 1-minute per-connection provider poll on the 0->1 transition. A
// blank wsID (the workspace list scope) no-ops in the manager.
func (s *Service) AcquireProviderPoll(
	wsID string,
) {
	s.providerPoll.Acquire(wsID)
}

// ReleaseProviderPoll drops one single-workspace WS subscriber for wsID,
// stopping the per-connection provider poll on the 1->0 transition.
func (s *Service) ReleaseProviderPoll(
	wsID string,
) {
	s.providerPoll.Release(wsID)
}

// Close cancels the root context shared by every watcher, LSP, and provider-poll
// goroutine, then stops and clears every resource still held by the managers so
// fsnotify file descriptors and LSP subprocesses are released promptly. It is
// idempotent and safe to call when nothing was ever started.
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.watchers.StopAll()
		s.lsps.StopAll()
		s.providerPoll.StopAll()
	})
}
