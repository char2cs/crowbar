package termwait

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const DefaultInterval = 2 * time.Second

const DefaultStallQuiet = 120 * time.Second

const DefaultDeliveryQuiet = 30 * time.Second

const DefaultMessageQuiet = 30 * time.Second

// DefaultIdleQuiet is how long a provider's own idle report is allowed to stand
// before the turn it belongs to is treated as one nothing will ever close.
//
// Far shorter than the screen-scraping quiet periods around it, because the
// signal is AUTHORITATIVE rather than heuristic: the provider said it is doing
// nothing. Measured against codex-cli 0.149.1, the gap between that report and
// the turn's own close on a healthy turn is sub-millisecond, so this is roughly
// four orders of magnitude of headroom.
const DefaultIdleQuiet = 5 * time.Second

type Runners interface {
	AllLive(ctx context.Context) ([]engineagents.Runner, error)
}

type Chats interface {
	GetChat(ctx context.Context, id string) (domain.Chat, error)
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

type Work interface {
	OpenWork(ctx context.Context, chatID string) (bool, error)
}

// Idle reports when the provider itself last said it was doing nothing, if
// nothing has closed the turn since. See turn/idle.go for why it is a latch the
// sweep reads rather than something acted on where it arrives.
type Idle interface {
	ProviderIdleSince(chatID string) (at time.Time, ok bool)
}

type Publish func(chatID, workspaceID string, wait domain.AgentTerminalWait)

type Stalled func(ctx context.Context, stall seam.Stall)

type Messages interface {
	UnfinishedSince(chatID string) (at time.Time, ok bool)

	AbandonMessage(ctx context.Context, chatID string) (bool, error)
}

// Liveness answers whether Crowbar still holds the live provider connection this
// runner's turn is riding on.
//
// It exists to keep the message-quiet heuristic off the one transport that does
// not need it. That heuristic infers "the CLI is gone" from an assistant message
// going quiet — the only thing a hooks/PTY provider gives you to infer it from.
// A connection-carried turn answers the same question directly, and its loss is
// reconciled on its own (runner/connloss.go), so silence there is a model
// thinking, not a death.
type Liveness interface {
	HasLiveAPIConnection(runnerID string) bool
}

type Deliveries interface {
	PendingDelivery(ctx context.Context, chatID string) (Delivery, bool)

	SettleDelivery(ctx context.Context, chatID, requestID string) (bool, error)
}

type Delivery struct {
	RequestID string

	RunnerID string
}

type Deps struct {
	Runners Runners
	Chats   Chats
	Choices Choices
	Screens Screens
	Prompts Prompts

	Notices Notices
	Work    Work
	OnStall Stalled

	Deliveries Deliveries

	Messages Messages

	Liveness Liveness

	Idle Idle

	Interval time.Duration

	StallQuiet time.Duration

	DeliveryQuiet time.Duration

	MessageQuiet time.Duration

	IdleQuiet time.Duration

	Now func() time.Time
}

type Detector interface {
	Wait(chatID string) domain.AgentTerminalWait

	Sweep(ctx context.Context, publish Publish)

	Run(ctx context.Context, publish Publish)
}

type detector struct {
	deps Deps

	mu sync.RWMutex

	state map[string]chatState
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

	since time.Time

	settled bool

	fired bool
}

func New(deps Deps) Detector {
	return &detector{deps: deps, state: make(map[string]chatState)}
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

func (d *detector) stallQuiet() time.Duration {
	if d.deps.StallQuiet > 0 {
		return d.deps.StallQuiet
	}
	return DefaultStallQuiet
}

func (d *detector) deliveryQuiet() time.Duration {
	if d.deps.DeliveryQuiet > 0 {
		return d.deps.DeliveryQuiet
	}
	return DefaultDeliveryQuiet
}

func (d *detector) messageQuiet() time.Duration {
	if d.deps.MessageQuiet > 0 {
		return d.deps.MessageQuiet
	}
	return DefaultMessageQuiet
}

func (d *detector) idleQuiet() time.Duration {
	if d.deps.IdleQuiet > 0 {
		return d.deps.IdleQuiet
	}
	return DefaultIdleQuiet
}

func (d *detector) now() time.Time {
	if d.deps.Now != nil {
		return d.deps.Now()
	}
	return time.Now()
}
