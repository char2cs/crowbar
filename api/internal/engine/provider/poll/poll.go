// Package poll provides on-view polling and background sweep scheduling
// for the provider engine. It is agnostic to the specific provider (GitHub/GitLab).
package poll

import (
	"context"
	"sync"
	"time"
)

// SweepTarget is the minimal info the sweep needs per workspace.
type SweepTarget struct {
	WSID      string
	RepoPath  string
	Branch    string
	HasOpenPR bool // only sweep if true
}

// PRInfoSnapshot mirrors types.PRInfo without creating an import cycle.
type PRInfoSnapshot struct {
	Number       int
	Status       string
	URL          string
	Title        string
	TargetBranch string
}

// ProviderStateSnapshot mirrors types.ProviderState for the sweep layer.
type ProviderStateSnapshot struct {
	Protected bool
	PR        *PRInfoSnapshot
}

// PollFn is the function the Sweeper calls per workspace target.
// It is provided by the engine layer to avoid import cycles.
type PollFn func(
	ctx context.Context,
	wsID string,
	repoPath string,
	branch string,
) (ProviderStateSnapshot, error)

// OnStateChangeFn is the callback invoked when provider state changes.
// Wave 3 will wire this to SyncProviderState on the Workspace aggregate.
type OnStateChangeFn func(
	wsID string,
	state ProviderStateSnapshot,
)

// sweeper holds the sweep scheduler state.
type sweeper struct {
	pollFn        PollFn
	onStateChange OnStateChangeFn
	interval      time.Duration
	mu            sync.Mutex
	lastState     map[string]ProviderStateSnapshot
}

// Sweeper schedules background provider polls.
type Sweeper interface {
	// Start launches the background sweep goroutine. It runs until ctx is done.
	// workspacesFn is called each tick to get the current workspace list.
	Start(
		ctx context.Context,
		workspacesFn func() []SweepTarget,
	)
}

// NewSweeper creates a Sweeper with the default 60-second interval.
func NewSweeper(
	pollFn PollFn,
	onStateChange OnStateChangeFn,
) Sweeper {
	return newSweeperWithInterval(pollFn, onStateChange, 60*time.Second)
}

// newSweeperWithInterval creates a Sweeper with a configurable interval.
// Used in tests to avoid waiting 60 seconds.
func newSweeperWithInterval(
	pollFn PollFn,
	onStateChange OnStateChangeFn,
	interval time.Duration,
) Sweeper {
	return &sweeper{
		pollFn:        pollFn,
		onStateChange: onStateChange,
		interval:      interval,
		lastState:     make(map[string]ProviderStateSnapshot),
	}
}

// Start launches the background sweep goroutine.
func (s *sweeper) Start(
	ctx context.Context,
	workspacesFn func() []SweepTarget,
) {
	go s.run(ctx, workspacesFn)
}

// run is the main sweep loop.
func (s *sweeper) run(
	ctx context.Context,
	workspacesFn func() []SweepTarget,
) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepOnce(ctx, workspacesFn())
		}
	}
}

// sweepOnce polls each target that has an open PR.
func (s *sweeper) sweepOnce(
	ctx context.Context,
	targets []SweepTarget,
) {
	for _, t := range targets {
		s.sweepTarget(ctx, t)
	}
}

// sweepTarget polls a single workspace target and notifies only on state change.
func (s *sweeper) sweepTarget(
	ctx context.Context,
	t SweepTarget,
) {
	if !t.HasOpenPR {
		return
	}

	state, err := s.pollFn(ctx, t.WSID, t.RepoPath, t.Branch)
	if err != nil {
		return
	}

	s.mu.Lock()
	changed := !statesEqual(s.lastState[t.WSID], state)
	if changed {
		s.lastState[t.WSID] = state
	}
	s.mu.Unlock()

	if changed {
		s.onStateChange(t.WSID, state)
	}
}

// statesEqual reports whether two ProviderStateSnapshot values are identical.
func statesEqual(
	a ProviderStateSnapshot,
	b ProviderStateSnapshot,
) bool {
	if a.Protected != b.Protected {
		return false
	}
	if (a.PR == nil) != (b.PR == nil) {
		return false
	}
	if a.PR == nil {
		return true
	}
	return *a.PR == *b.PR
}
