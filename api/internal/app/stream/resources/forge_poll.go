package resources

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

type ProviderPoller interface {
	PollWorkspace(
		ctx context.Context,
		wsID string,
	) error
}

type ForgePoll struct {
	root     context.Context
	interval time.Duration
	poll     ProviderPoller
	mu       sync.Mutex
	cancels  map[string]context.CancelFunc
	closed   bool

	runners sync.WaitGroup

	ticks     <-chan time.Time
	cycleDone chan<- struct{}
}

const pollTimeout = 30 * time.Second

func NewForgePoll(
	root context.Context,
	interval time.Duration,
	poll ProviderPoller,
) *ForgePoll {
	return &ForgePoll{
		root:     root,
		interval: interval,
		poll:     poll,
		cancels:  make(map[string]context.CancelFunc),
	}
}

func (m *ForgePoll) Start(
	wsID string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errClosed
	}
	ctx, cancel := context.WithCancel(m.root)
	m.cancels[wsID] = cancel
	m.runners.Add(1)
	go m.run(ctx, cancel, wsID)
	return nil
}

func (m *ForgePoll) Stop(
	wsID string,
) {
	m.mu.Lock()
	cancel, ok := m.cancels[wsID]
	delete(m.cancels, wsID)
	m.mu.Unlock()
	if ok {
		cancel()
	}
}

func (m *ForgePoll) Close() {
	m.mu.Lock()
	m.closed = true
	cancels := m.cancels
	m.cancels = make(map[string]context.CancelFunc)
	m.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	m.runners.Wait()
}

func (m *ForgePoll) run(
	ctx context.Context,
	cancel context.CancelFunc,
	wsID string,
) {
	defer m.runners.Done()
	defer cancel()
	defer safego.Recover("resources.forgePoll.run")
	select {
	case <-ctx.Done():
		return
	default:
		m.pollTick(ctx, wsID)
		m.notifyCycle(ctx)
	}

	tickC, stopTicker := m.tickSource()
	defer stopTicker()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tickC:
			m.pollTick(ctx, wsID)
			m.notifyCycle(ctx)
		}
	}
}

func (m *ForgePoll) tickSource() (<-chan time.Time, func()) {
	m.mu.Lock()
	ticks := m.ticks
	m.mu.Unlock()

	if ticks != nil {
		return ticks, func() {}
	}
	ticker := time.NewTicker(m.interval)
	return ticker.C, ticker.Stop
}

func (m *ForgePoll) notifyCycle(
	ctx context.Context,
) {
	m.mu.Lock()
	done := m.cycleDone
	m.mu.Unlock()

	if done == nil {
		return
	}
	select {
	case done <- struct{}{}:
	case <-ctx.Done():
	}
}

func (m *ForgePoll) pollTick(
	ctx context.Context,
	wsID string,
) {
	m.pollTickWithTimeout(ctx, wsID, pollTimeout)
}

func (m *ForgePoll) pollTickWithTimeout(
	ctx context.Context,
	wsID string,
	timeout time.Duration,
) {
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := m.poll.PollWorkspace(pollCtx, wsID); err != nil && !errors.Is(err, context.Canceled) {
		slog.WarnContext(ctx, "resources: forge poll", "ws", wsID, "err", err)
	}
}
