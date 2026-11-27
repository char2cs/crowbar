package poll

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

const GlobalCronInterval = 5 * time.Minute

const PerConnectionInterval = 1 * time.Minute

const perTargetTimeout = 30 * time.Second

type SweepTarget struct {
	WSID      string
	RepoPath  string
	Branch    string
	HasOpenPR bool
}

type PRInfoSnapshot struct {
	Number       int
	Status       string
	URL          string
	Title        string
	TargetBranch string
}

type ProviderStateSnapshot struct {
	Protected bool
	PR        *PRInfoSnapshot
}

type PollFn func(
	ctx context.Context,
	wsID string,
	repoPath string,
	branch string,
) (ProviderStateSnapshot, error)

type OnStateChangeFn func(
	wsID string,
	state ProviderStateSnapshot,
)

type sweeper struct {
	pollFn        PollFn
	onStateChange OnStateChangeFn
	interval      time.Duration
	targetTimeout time.Duration
	mu            sync.Mutex
	lastState     map[string]ProviderStateSnapshot
}

type Sweeper interface {
	Start(
		ctx context.Context,
		workspacesFn func() []SweepTarget,
	)
}

func NewSweeper(
	pollFn PollFn,
	onStateChange OnStateChangeFn,
) Sweeper {
	return newSweeperWithInterval(pollFn, onStateChange, GlobalCronInterval)
}

func newSweeperWithInterval(
	pollFn PollFn,
	onStateChange OnStateChangeFn,
	interval time.Duration,
) Sweeper {
	return &sweeper{
		pollFn:        pollFn,
		onStateChange: onStateChange,
		interval:      interval,
		targetTimeout: perTargetTimeout,
		lastState:     make(map[string]ProviderStateSnapshot),
	}
}

func (s *sweeper) Start(
	ctx context.Context,
	workspacesFn func() []SweepTarget,
) {
	go s.run(ctx, workspacesFn)
}

func (s *sweeper) run(
	ctx context.Context,
	workspacesFn func() []SweepTarget,
) {
	defer safego.Recover("provider.poll.run")
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

func (s *sweeper) sweepOnce(
	ctx context.Context,
	targets []SweepTarget,
) {
	for _, t := range targets {
		s.sweepTarget(ctx, t)
	}
	s.pruneLastState(targets)
}

func (s *sweeper) pruneLastState(targets []SweepTarget) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lastState) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		seen[t.WSID] = struct{}{}
	}
	for wsID := range s.lastState {
		if _, ok := seen[wsID]; !ok {
			delete(s.lastState, wsID)
		}
	}
}

func (s *sweeper) sweepTarget(
	ctx context.Context,
	t SweepTarget,
) {
	if !t.HasOpenPR {
		return
	}

	timeout := s.targetTimeout
	if timeout <= 0 {
		timeout = perTargetTimeout
	}
	targetCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	state, err := s.pollFn(targetCtx, t.WSID, t.RepoPath, t.Branch)
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
