package answerdesk

import (
	"sync"
	"time"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const MaxPayloadBytes = 128 << 10

type HookAnswer struct {
	Stdout []byte
}

type Prompt struct {
	ChoiceID string
	ChatID   string
	RunnerID string
	Event    string
	Raw      []byte
	Keys     engineagents.AnswerCapability
}

type Slot struct {
	Prompt

	done   chan struct{}
	stdout []byte
	once   sync.Once

	decidedAt time.Time
	spent     bool
	reap      *time.Timer
	answering *claim
}

type claim struct {
	slot *Slot
	done chan struct{}
}

func (s *Slot) Settled() <-chan struct{} { return s.done }

func (s *Slot) settle(stdout []byte) {
	s.once.Do(func() {
		s.stdout = stdout
		close(s.done)
	})
}
