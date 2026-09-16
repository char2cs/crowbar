package turn

import (
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// idleLatch remembers that a provider said it was doing nothing, and when.
//
// It exists because the signal cannot be acted on directly. Measured live against
// codex-cli 0.149.1, thread/status/changed(idle) arrives IMMEDIATELY BEFORE
// turn/completed on a perfectly healthy turn — sub-millisecond — so a reconcile
// that fired on it would abandon every turn a moment before it ended properly,
// salvaging and closing work that was about to be recorded correctly.
//
// What the signal is genuinely good for is the case nothing else can reach: a
// turn whose close is never sent at all. codex.yaml records that happening live,
// and the only thing covering it today is a 120s screen-scraping heuristic that
// needs a declared notice visible on a PTY, or a half-streamed message that has
// gone quiet — neither of which exists for a turn that only reasoned.
//
// So: the turn-close CLEARS this latch, and a latch still armed when the sweep
// next looks is a turn nothing is going to close, on a provider that has said so
// itself.
type idleLatch struct {
	mu    sync.Mutex
	since map[string]time.Time
}

func newIdleLatch() *idleLatch {
	return &idleLatch{since: make(map[string]time.Time)}
}

// arm records that the provider reported idle, keeping the EARLIEST report. A
// provider that repeats itself must not keep pushing the deadline out.
func (l *idleLatch) arm(chatID string, at time.Time) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, already := l.since[chatID]; already {
		return
	}
	l.since[chatID] = at
}

func (l *idleLatch) clear(chatID string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.since, chatID)
}

func (l *idleLatch) at(chatID string) (time.Time, bool) {
	if l == nil {
		return time.Time{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	t, ok := l.since[chatID]
	return t, ok
}

// recordIdle arms the latch. It closes nothing, on purpose — see idleLatch.
func (t *Turns) recordIdle(chat domain.Chat) {
	t.idle.arm(chat.ID, time.Now())
}

// ProviderIdleSince reports when the provider last said it was doing nothing, if
// nothing has closed the turn since. The terminal-wait sweep is its only reader.
func (t *Turns) ProviderIdleSince(chatID string) (time.Time, bool) {
	return t.idle.at(chatID)
}

// ForgetIdle disarms the latch. Every path that opens or closes a turn calls it:
// an armed latch means "the provider says it is done and Crowbar has not noticed",
// and the moment Crowbar notices — or a new turn begins — that is no longer true.
func (t *Turns) ForgetIdle(chatID string) { t.idle.clear(chatID) }
