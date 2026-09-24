package runner

import (
	"context"
	"errors"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// Snapshotter is where a lifecycle change no aggregate event carries is
// announced: the chat's phase, and a runner's in-memory process state.
type Snapshotter interface {
	Touch(ctx context.Context, chatID string)
	TouchRunner(ctx context.Context, runnerID string)
}

// phases is the lifecycle operation each chat is in the middle of, set by the
// operation holding the chat's spawn gate. Absent means none: the chat's phase
// is then derived from placement (live or dormant) by the snapshot owner.
type phases struct {
	mu     sync.Mutex
	byChat map[string]string
}

func newPhases() *phases { return &phases{byChat: map[string]string{}} }

// Phase implements snapshot.Runtime.
func (rs *Runners) Phase(chatID string) string {
	if rs.phases == nil {
		return ""
	}
	rs.phases.mu.Lock()
	defer rs.phases.mu.Unlock()
	return rs.phases.byChat[chatID]
}

// enterPhase marks chatID as being in phase for the duration of a gated
// operation, announces it, and returns the func that ends it (and announces
// that). Nested entries are not a thing: the spawn gate admits one operation
// per chat at a time.
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

// replacementPhase is what replacing chatID's CLI looks like from outside:
// switching while one is live, starting while the chat is dormant.
func (rs *Runners) replacementPhase(ctx context.Context, chatID string) string {
	if _, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID); errors.Is(err, agentrunner.ErrNotFound) {
		return snapshot.PhaseStarting
	}
	return snapshot.PhaseSwitching
}

func (rs *Runners) touch(ctx context.Context, chatID string) {
	if rs.snapshots != nil {
		rs.snapshots.Touch(ctx, chatID)
	}
}

func (rs *Runners) touchRunner(ctx context.Context, runnerID string) {
	if rs.snapshots != nil {
		rs.snapshots.TouchRunner(ctx, runnerID)
	}
}
