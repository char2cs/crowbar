package drain

import (
	"context"
	"sync"
)

type Gate struct {
	mu       sync.Mutex
	n        int
	draining bool

	idle chan struct{}

	hold    chan struct{}
	parked  int
	settled chan struct{}
}

func New() *Gate {
	return &Gate{}
}

func (g *Gate) Enter() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.draining {
		return false
	}
	g.n++
	return true
}

func (g *Gate) Leave() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.n--
	if g.n == 0 && g.idle != nil {
		close(g.idle)
		g.idle = nil
	}
	g.wakeSettled()
}

func (g *Gate) wakeSettled() {
	if g.n == g.parked && g.settled != nil {
		close(g.settled)
		g.settled = nil
	}
}

func (g *Gate) Hold() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.hold == nil {
		g.hold = make(chan struct{})
	}
}

func (g *Gate) Release() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.hold != nil {
		close(g.hold)
		g.hold = nil
		g.parked = 0
	}
}

func (g *Gate) Proceed(
	ctx context.Context,
) bool {
	g.mu.Lock()
	hold := g.hold
	if hold == nil {
		g.mu.Unlock()
		return true
	}
	g.parked++
	g.wakeSettled()
	g.mu.Unlock()

	select {
	case <-hold:
		return true
	case <-ctx.Done():
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.hold == hold {
		g.parked--
	}
	return false
}

func (g *Gate) Parked() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.parked
}

func (g *Gate) WaitRunning(
	ctx context.Context,
) {
	g.mu.Lock()
	if g.n == g.parked {
		g.mu.Unlock()
		return
	}
	if g.settled == nil {
		g.settled = make(chan struct{})
	}
	settled := g.settled
	g.mu.Unlock()

	select {
	case <-settled:
	case <-ctx.Done():
	}
}

func (g *Gate) WaitIdle(
	ctx context.Context,
) {
	g.mu.Lock()
	if g.n == 0 {
		g.mu.Unlock()
		return
	}
	if g.idle == nil {
		g.idle = make(chan struct{})
	}
	idle := g.idle
	g.mu.Unlock()

	select {
	case <-idle:
	case <-ctx.Done():
	}
}

func (g *Gate) Wait(
	ctx context.Context,
) {
	g.mu.Lock()
	g.draining = true
	g.mu.Unlock()

	g.WaitIdle(ctx)
}
