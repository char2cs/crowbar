package answerdesk

import (
	"sync"
	"time"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// MaxPayloadBytes caps the raw provider payload a prompt may carry into the desk.
// The payload is held in memory for as long as a person is thinking, and it is
// handed straight back to the provider to render the answer against, so a prompt
// bigger than this is left to the provider's own UI instead.
const MaxPayloadBytes = 128 << 10

// PendingAnswer is what a blocked relay is told: which prompt it is parked on,
// and how long it may wait for a person before it must print nothing and let the
// provider fall back to its own UI.
type PendingAnswer struct {
	ChoiceID string
	Wait     time.Duration
}

// HookAnswer is what a relay prints. An empty Stdout means nobody answered in
// time — the signal to leave the prompt to the provider.
type HookAnswer struct {
	Stdout []byte
}

// Prompt is everything the desk needs to hold one provider question open: which
// question it is, whose it is, and the raw payload the provider will want back
// when the answer is rendered.
type Prompt struct {
	ChoiceID string
	ChatID   string
	RunnerID string
	// Event is the canonical hook event the prompt arrived on. The provider needs
	// it to render an answer, because the answer's shape is per-event.
	Event string
	Raw   []byte
	// Keys is what this provider can express: which decision keys it accepts, and
	// how long its relay is willing to block.
	Keys engineagents.AnswerCapability
}

// Slot is one relay currently BLOCKED on a human decision.
//
// It is in memory and can never be otherwise: a slot describes a live hook
// process holding a live provider gate open, so a slot that survived a restart
// would be a promise to a process that no longer exists.
type Slot struct {
	Prompt

	done   chan struct{}
	stdout []byte
	once   sync.Once

	// decidedAt is when a verdict landed. A decided slot lingers for the desk's
	// retention so a relay that has not asked yet still finds its answer.
	decidedAt time.Time
	// spent is set the moment the slot leaves the desk, so a second claim of one
	// verdict is impossible however the callers race.
	spent bool
	reap  *time.Timer
}

// Settled is closed once the slot has a verdict or has been given up on. A closed
// channel rather than a value is the release, so any number of waiters wake and a
// waiter that arrives late never blocks.
func (s *Slot) Settled() <-chan struct{} { return s.done }

func (s *Slot) settle(stdout []byte) {
	s.once.Do(func() {
		s.stdout = stdout
		close(s.done)
	})
}
