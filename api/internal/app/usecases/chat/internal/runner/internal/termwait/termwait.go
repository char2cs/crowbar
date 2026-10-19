package termwait

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const DefaultInterval = 2 * time.Second

type Runners interface {
	AllLive(ctx context.Context) ([]engineagents.Runner, error)
}

// Work reports what a chat's ledger folds: whether it is working, and whether
// a prompt still awaits its provider.
type Work interface {
	ChatWork(ctx context.Context, chatID string) (busy, awaiting bool, err error)
}

type Choices interface {
	PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)
}

type Screens interface {
	Screen(sessionID string, since uint64) (text string, gen uint64, changed bool)
}

type Prompts interface {
	MatchTerminalPrompt(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalPrompt, bool)
}

type Notices interface {
	MatchTerminalNotice(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalNotice, bool)
}

type Publish func(chatID, workspaceID string, wait domain.AgentTerminalWait)

// OnNotice is told of a notice that ends a turn, the moment it appears on
// the screen of a runner whose chat is busy or awaiting a prompt.
type OnNotice func(ctx context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice)

type Deps struct {
	Runners  Runners
	Work     Work
	Choices  Choices
	Screens  Screens
	Prompts  Prompts
	Notices  Notices
	OnNotice OnNotice
	Interval time.Duration
}

type Detector interface {
	Wait(chatID string) domain.AgentTerminalWait

	Sweep(ctx context.Context, publish Publish)

	Run(ctx context.Context, publish Publish)

	// Wake resumes a Run loop parked because no runner was live.
	Wake()
}

type detector struct {
	deps Deps

	mu sync.RWMutex

	state map[string]chatState

	wake chan struct{}
}

type chatState struct {
	workspaceID string
	screen      screenCache

	published domain.AgentTerminalWait
}

type screenCache struct {
	session string
	gen     uint64

	text string

	matched domain.AgentTerminalWait

	notice engineagents.TerminalNotice
}

func New(deps Deps) Detector {
	return &detector{deps: deps, state: make(map[string]chatState), wake: make(chan struct{}, 1)}
}

func (d *detector) Wait(chatID string) domain.AgentTerminalWait {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state[chatID].published
}

func (d *detector) interval() time.Duration {
	if d.deps.Interval > 0 {
		return d.deps.Interval
	}
	return DefaultInterval
}
