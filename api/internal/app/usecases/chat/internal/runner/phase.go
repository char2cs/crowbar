package runner

import (
	"context"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
)

// Snapshotter is where a lifecycle change no aggregate event carries is
// announced: the chat's phase, and its runner starting, reporting or exiting.
type Snapshotter interface {
	Touch(ctx context.Context, chatID string)
	ApplyRunner(ctx context.Context, chatID, runnerID, kind string)
}

// phases is the lifecycle operation each chat is in the middle of, set by the
// user-initiated operation holding the chat. Absent means none: the chat's phase
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

// enterPhase marks chatID as being in phase for the duration of an operation
// holding the chat, announces it, and returns the func that ends it (and
// announces that).
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

// announceRunner publishes chatID's snapshot for its runner's lifecycle event.
func (rs *Runners) announceRunner(ctx context.Context, chatID, runnerID, kind string) {
	if rs.snapshots != nil {
		rs.snapshots.ApplyRunner(ctx, chatID, runnerID, kind)
	}
}
