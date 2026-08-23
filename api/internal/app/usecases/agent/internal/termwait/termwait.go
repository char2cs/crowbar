package termwait

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const DefaultInterval = 2 * time.Second

const DefaultStallQuiet = 120 * time.Second

const DefaultDeliveryQuiet = 30 * time.Second

const DefaultMessageQuiet = 30 * time.Second

type Runners interface {
	AllLive(ctx context.Context) ([]domain.AgentRunner, error)
}

type Chats interface {
	GetChat(ctx context.Context, id string) (domain.AgentChat, error)
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

type Publish func(chatID, workspaceID string, wait domain.AgentTerminalWait)

type Stall struct {
	ChatID      string
	WorkspaceID string
	ProviderID  string
	RunnerID    string
	SessionID   string

	Notice engineagents.TerminalNotice
}

type Stalled func(ctx context.Context, stall Stall)

type Messages interface {
	UnfinishedSince(chatID string) (at time.Time, ok bool)

	AbandonMessage(ctx context.Context, chatID string) (bool, error)
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

	Interval time.Duration

	StallQuiet time.Duration

	DeliveryQuiet time.Duration

	MessageQuiet time.Duration

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

func (d *detector) now() time.Time {
	if d.deps.Now != nil {
		return d.deps.Now()
	}
	return time.Now()
}
