package runner

import (
	"context"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
)

type Snapshotter interface {
	Touch(ctx context.Context, chatID string)
}

type phases struct {
	mu     sync.Mutex
	byChat map[string]string
}

func newPhases() *phases { return &phases{byChat: map[string]string{}} }

func (rs *Runners) Phase(chatID string) string {
	if rs.phases == nil {
		return ""
	}
	rs.phases.mu.Lock()
	defer rs.phases.mu.Unlock()
	return rs.phases.byChat[chatID]
}

func (rs *Runners) enterPhase(ctx context.Context, chatID, phase string) func() {
	if rs.phases == nil {
		return func() {}
	}
	rs.phases.mu.Lock()
	rs.phases.byChat[chatID] = phase
	rs.phases.mu.Unlock()
	rs.touch(ctx, chatID)
	return func() {
		rs.phases.mu.Lock()
		delete(rs.phases.byChat, chatID)
		rs.phases.mu.Unlock()
		rs.touch(context.WithoutCancel(ctx), chatID)
	}
}

func (rs *Runners) replacementPhase(chatID string) string {
	if _, live := rs.live.byChat(chatID); !live {
		return snapshot.PhaseStarting
	}
	return snapshot.PhaseSwitching
}

func (rs *Runners) touch(ctx context.Context, chatID string) {
	if rs.snapshots != nil {
		rs.snapshots.Touch(ctx, chatID)
	}
}
