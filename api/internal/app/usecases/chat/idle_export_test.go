package chat

import (
	"context"
	"time"
)

// ProviderIdleLatch reports whether a provider's own "I am doing nothing"
// report is currently armed for chatID, and when it landed.
//
// Test-only surface: this file compiles only under `go test`. The latch has
// exactly one production reader — the terminal-wait sweep, behind
// internal/runner, which an external test of this package cannot construct —
// so a test proving that a provider's own idle report actually ARMED it has to
// read it here.
func ProviderIdleLatch(u TurnUsecase, chatID string) (time.Time, bool) {
	return u.(*Usecase).turns.ProviderIdleSince(chatID)
}

// CloseIdleTurn runs the exact close the terminal-wait sweep performs once an
// armed latch outlives termwait.DefaultIdleQuiet (evaluate.go's
// providerSaysItIsIdle → Messages.AbandonMessage). Test-only surface, same as
// above: the 5s fuse itself is driven off a fake clock inside termwait's own
// package; what an external test needs is to run its CONSEQUENCE against a
// real, wedged chat.
func CloseIdleTurn(ctx context.Context, u TurnUsecase, chatID string) (bool, error) {
	return u.(*Usecase).turns.AbandonMessage(ctx, chatID)
}
