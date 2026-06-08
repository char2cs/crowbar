// Package handlers holds the gin handlers backing the agentrun endpoint.
package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentRunRepo is the agent-run repository surface the handlers need.
type AgentRunRepo interface {
	Create(
		ctx context.Context,
		id string,
		wsID string,
		chatID string,
		now time.Time,
	) (domain.AgentRun, error)
	ListRunning(
		ctx context.Context,
	) ([]domain.AgentRun, error)
	MarkRunning(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Complete(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Fail(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
}

// Handlers holds the gin handler methods for the agentrun endpoint.
type Handlers struct {
	repo AgentRunRepo
}

// New constructs a Handlers backed by the given AgentRunRepo.
func New(repo AgentRunRepo) *Handlers {
	return &Handlers{repo: repo}
}
