package activity

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const maxOCCAttempts = 8

type TurnInput struct {
	ChatID     string
	TurnID     string
	Role       string
	ProviderID string
	RunnerID   string
	SessionID  string
	Text       string
	Effort     string
	// ItemIndex — see commands.CloseTurn.ItemIndex. Only meaningful on CloseTurn.
	ItemIndex int
	Now       time.Time
}

type ToolInput struct {
	ChatID  string
	ToolID  string
	Name    string
	Target  string
	Request []byte
	Now     time.Time
}

type ToolResultInput struct {
	ChatID string
	ToolID string
	Name   string
	Target string
	Result []byte
	Status string

	Error      string
	DurationMS int
	Now        time.Time
}

type ChoiceInput struct {
	ChatID   string
	ChoiceID string
	Kind     string
	PromptID string
	ToolName string
	Title    string
	Question string
	Mode     string
	Multi    bool
	Options  []domain.ActivityChoiceOption

	Questions []domain.ActivityChoiceQuestion
	Schema    string
	Now       time.Time
}

const maxToolErrorBytes = 2 << 10

type EventStore interface {
	AppendTurn(ctx context.Context, in TurnInput) error

	OpenTurn(ctx context.Context, in TurnInput) error

	CloseTurn(ctx context.Context, in TurnInput) error

	Abandon(ctx context.Context, chatID string, now time.Time) error

	InvokeTool(ctx context.Context, in ToolInput) error
	CompleteTool(ctx context.Context, in ToolResultInput) error

	StartSubagent(ctx context.Context, chatID, subagentID, agentType string, now time.Time) error
	StopSubagent(ctx context.Context, chatID, subagentID, agentType string, now time.Time) error

	Interrupt(ctx context.Context, chatID, id, kind, detail string, now time.Time) error
	ResolveInterruption(ctx context.Context, chatID, id, kind, detail string, now time.Time) error

	OpenChoice(ctx context.Context, in ChoiceInput) error

	ResolveChoice(ctx context.Context, chatID, choiceID, resolution string, now time.Time) error

	AnswerChoice(ctx context.Context, chatID, choiceID string, optionIDs []string, auto bool, now time.Time) error

	Turns(ctx context.Context, chatID string, after, before int64, limit int) ([]domain.ActivityTurn, error)

	TurnsBefore(ctx context.Context, chatID string, before int64, limit int) ([]domain.ActivityTurn, error)
	TurnsSince(ctx context.Context, chatID string, cut time.Time) ([]domain.ActivityTurn, error)
	CountTurns(ctx context.Context, chatID string) (int64, error)

	LastTurnAt(ctx context.Context, chatID, providerID string) (time.Time, bool, error)
	LastTurnForSession(ctx context.Context, chatID, providerID, sessionID string) (time.Time, bool, error)
	HasTurnAtOrAfter(ctx context.Context, chatID, providerID string, since time.Time) (bool, error)

	ToolCalls(ctx context.Context, chatID string, after int64, limit int) ([]domain.ActivityToolCall, error)
	ToolCallsBefore(ctx context.Context, chatID string, before int64, limit int) ([]domain.ActivityToolCall, error)
	Subagents(ctx context.Context, chatID string) ([]domain.ActivitySubagent, error)
	Interruptions(ctx context.Context, chatID string) ([]domain.ActivityInterruption, error)

	Choices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)

	PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)
	RecentToolCalls(ctx context.Context, chatIDs []string, since time.Time, limit int) ([]domain.ActivityToolCall, error)

	Payload(ctx context.Context, ref string) ([]byte, error)

	Forget(ctx context.Context, chatID string) error
}

type eventSourced struct {
	ax    asynx.Asynx[domain.ChatActivity]
	store *store.Store

	// snapshotInterval and snapshotCounter exist purely as a test seam (see
	// SetSnapshotIntervalForTest in export_test.go). asynx defers snapshot
	// cadence entirely to each command's own ShouldSnapshot(), which most
	// high-frequency commands here (InvokeTool, CompleteTool, OpenChoice...)
	// correctly leave off the hot path in production. A test that drives
	// hundreds of such commands at one aggregate with no snapshotting
	// command interleaved pays full cold-replay-from-version-1 on every one
	// (O(n^2)); forcing an occasional snapshot bounds that without changing
	// any observable state, since snapshot+delta replay must always
	// reconstruct the same state as a full replay.
	snapshotInterval atomic.Int64
	snapshotCounter  atomic.Int64
}

// snapshotForcingCommand overrides ShouldSnapshot to true while forwarding
// every other Command method to the wrapped command unchanged.
type snapshotForcingCommand struct {
	asynxModels.Command[domain.ChatActivity]
}

func (snapshotForcingCommand) ShouldSnapshot() bool { return true }

// maybeForceSnapshot wraps cmd so it snapshots once every snapshotInterval
// dispatched commands, when an interval has been set (test-only; zero is the
// production default and returns cmd unchanged).
func (r *eventSourced) maybeForceSnapshot(
	cmd asynxModels.Command[domain.ChatActivity],
) asynxModels.Command[domain.ChatActivity] {
	n := r.snapshotInterval.Load()
	if n <= 0 {
		return cmd
	}
	if r.snapshotCounter.Add(1)%n != 0 {
		return cmd
	}
	return snapshotForcingCommand{cmd}
}

func NewEventSourced(
	ax asynx.Asynx[domain.ChatActivity],
	es asynxModels.Store,
	storeDB *gormdb.DB,
	contentRoot string,
) (EventStore, error) {
	st, err := store.New(storeDB, contentRoot, ax, es)
	if err != nil {
		return nil, fmt.Errorf("agentactivity: store: %w", err)
	}
	return &eventSourced{ax: ax, store: st}, nil
}

func (r *eventSourced) send(ctx context.Context, cmd asynxModels.Command[domain.ChatActivity]) error {
	return r.dispatch(ctx, r.ax.Send, r.maybeForceSnapshot(cmd))
}

func (r *eventSourced) sendWait(ctx context.Context, cmd asynxModels.Command[domain.ChatActivity]) error {
	return r.dispatch(ctx, r.ax.SendWait, r.maybeForceSnapshot(cmd))
}

type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.ChatActivity],
) (asynxModels.Event[domain.ChatActivity], error)

func (r *eventSourced) dispatch(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.ChatActivity],
) error {
	var lastErr error
	for range maxOCCAttempts {
		_, err := send(ctx, cmd)
		if err == nil {
			return nil
		}
		switch {
		case errors.Is(err, asynxModels.ErrValidation):
			return err
		case errors.Is(err, asynxModels.ErrQueueFull):
			return fmt.Errorf("agentactivity: send: %w", apperr.ErrUnavailable)
		case errors.Is(err, asynxModels.ErrPipelineFailed):
			lastErr = err
		default:
			return err
		}
	}
	return lastErr
}

func (r *eventSourced) AppendTurn(ctx context.Context, in TurnInput) error {
	return r.sendWait(ctx, commands.AppendTurn{
		ChatID: in.ChatID, TurnID: in.TurnID, Role: in.Role,
		ProviderID: in.ProviderID, RunnerID: in.RunnerID, SessionID: in.SessionID,
		Text: in.Text, Effort: in.Effort, Now: in.Now,
	})
}

func (r *eventSourced) OpenTurn(ctx context.Context, in TurnInput) error {
	return r.sendWait(ctx, commands.OpenTurn{
		ChatID: in.ChatID, TurnID: in.TurnID,
		ProviderID: in.ProviderID, RunnerID: in.RunnerID, SessionID: in.SessionID,
		Now: in.Now,
	})
}

func (r *eventSourced) CloseTurn(ctx context.Context, in TurnInput) error {
	return r.sendWait(ctx, commands.CloseTurn{
		ChatID: in.ChatID, TurnID: in.TurnID,
		ProviderID: in.ProviderID, RunnerID: in.RunnerID, SessionID: in.SessionID,
		Text: in.Text, Effort: in.Effort, ItemIndex: in.ItemIndex, Now: in.Now,
	})
}

func (r *eventSourced) Abandon(ctx context.Context, chatID string, now time.Time) error {
	return r.sendWait(ctx, commands.Abandon{ChatID: chatID, Now: now})
}

func (r *eventSourced) InvokeTool(ctx context.Context, in ToolInput) error {
	ref, err := r.store.Content().Put(in.Request)
	if err != nil {
		ref = ""
	}
	return r.send(ctx, commands.InvokeTool{
		ChatID: in.ChatID, ToolID: in.ToolID, Name: in.Name, Target: in.Target,
		RequestRef: ref, Now: in.Now,
	})
}

// CompleteTool uses sendWait, not send, for the same reason OpenChoice does:
// closing the last piece of open work is immediately followed by a caller
// asking the READ MODEL whether any work is still open (turn.restateAsyncWork,
// via OpenWork). On send, that question was asked before this close had
// projected, so the answer was the state one event ago — still open — the
// restate was skipped as a no-op, and nothing ever cleared the spinner.
//
// The aggregate cannot answer it instead: CloseTurn nils Tools and Subagents,
// so work that outlives its turn is only ever visible in the projection.
// Opening work (InvokeTool, StartSubagent) owes no such barrier and keeps the
// unblocked hot path — nothing reads back to decide anything.
func (r *eventSourced) CompleteTool(ctx context.Context, in ToolResultInput) error {
	ref, err := r.store.Content().Put(in.Result)
	if err != nil {
		ref = ""
	}
	return r.sendWait(ctx, commands.CompleteTool{
		ChatID: in.ChatID, ToolID: in.ToolID, Name: in.Name, Target: in.Target,
		ResultRef: ref, Status: in.Status, Error: truncate(in.Error, maxToolErrorBytes),
		DurationMS: in.DurationMS, Now: in.Now,
	})
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// OpenChoice uses sendWait, not send: a caller that immediately follows this
// with AnswerChoice for the same choice (auto-approve) must see it durably
// settled first, or AnswerChoice's Validate reads a state where the choice
// never opened and rejects it as no longer pending.
func (r *eventSourced) OpenChoice(ctx context.Context, in ChoiceInput) error {
	return r.sendWait(ctx, commands.OpenChoice{
		ChatID: in.ChatID, ChoiceID: in.ChoiceID, Kind: in.Kind,
		PromptID: in.PromptID, ToolName: in.ToolName,
		Title: in.Title, Question: in.Question, Mode: in.Mode, Multi: in.Multi,
		Options: in.Options, Questions: in.Questions, Schema: in.Schema, Now: in.Now,
	})
}

func (r *eventSourced) ResolveChoice(
	ctx context.Context, chatID, choiceID, resolution string, now time.Time,
) error {
	return r.send(ctx, commands.ResolveChoice{
		ChatID: chatID, ChoiceID: choiceID, Resolution: resolution, Now: now,
	})
}

func (r *eventSourced) AnswerChoice(
	ctx context.Context, chatID, choiceID string, optionIDs []string, auto bool, now time.Time,
) error {
	return r.send(ctx, commands.AnswerChoice{
		ChatID: chatID, ChoiceID: choiceID, OptionIDs: optionIDs, Auto: auto, Now: now,
	})
}

func (r *eventSourced) StartSubagent(
	ctx context.Context, chatID, subagentID, agentType string, now time.Time,
) error {
	return r.send(ctx, commands.StartSubagent{
		ChatID: chatID, SubagentID: subagentID, AgentType: agentType, Now: now,
	})
}

// StopSubagent is CompleteTool's twin and sendWait for the same reason: it is
// the other close turn.restateAsyncWork reads back through OpenWork.
func (r *eventSourced) StopSubagent(
	ctx context.Context, chatID, subagentID, agentType string, now time.Time,
) error {
	return r.sendWait(ctx, commands.StopSubagent{
		ChatID: chatID, SubagentID: subagentID, AgentType: agentType, Now: now,
	})
}

func (r *eventSourced) Interrupt(
	ctx context.Context, chatID, id, kind, detail string, now time.Time,
) error {
	return r.send(ctx, commands.Interrupt{
		ChatID: chatID, ID: id, Kind: kind, Detail: detail, Now: now,
	})
}

func (r *eventSourced) ResolveInterruption(
	ctx context.Context, chatID, id, kind, detail string, now time.Time,
) error {
	return r.send(ctx, commands.ResolveInterruption{
		ChatID: chatID, ID: id, Kind: kind, Detail: detail, Now: now,
	})
}

func (r *eventSourced) Turns(
	ctx context.Context, chatID string, after, before int64, limit int,
) ([]domain.ActivityTurn, error) {
	return r.store.Turns(ctx, chatID, after, before, limit)
}

func (r *eventSourced) TurnsBefore(
	ctx context.Context, chatID string, before int64, limit int,
) ([]domain.ActivityTurn, error) {
	return r.store.TurnsBefore(ctx, chatID, before, limit)
}

func (r *eventSourced) TurnsSince(
	ctx context.Context, chatID string, cut time.Time,
) ([]domain.ActivityTurn, error) {
	return r.store.TurnsSince(ctx, chatID, cut)
}

func (r *eventSourced) CountTurns(ctx context.Context, chatID string) (int64, error) {
	return r.store.CountTurns(ctx, chatID)
}

func (r *eventSourced) LastTurnAt(
	ctx context.Context, chatID, providerID string,
) (time.Time, bool, error) {
	return r.store.LastTurnAt(ctx, chatID, providerID)
}

func (r *eventSourced) LastTurnForSession(
	ctx context.Context, chatID, providerID, sessionID string,
) (time.Time, bool, error) {
	return r.store.LastTurnForSession(ctx, chatID, providerID, sessionID)
}

func (r *eventSourced) HasTurnAtOrAfter(
	ctx context.Context, chatID, providerID string, since time.Time,
) (bool, error) {
	return r.store.HasTurnAtOrAfter(ctx, chatID, providerID, since)
}

func (r *eventSourced) ToolCalls(
	ctx context.Context, chatID string, after int64, limit int,
) ([]domain.ActivityToolCall, error) {
	return r.store.ToolCalls(ctx, chatID, after, limit)
}

func (r *eventSourced) ToolCallsBefore(
	ctx context.Context, chatID string, before int64, limit int,
) ([]domain.ActivityToolCall, error) {
	return r.store.ToolCallsBefore(ctx, chatID, before, limit)
}

func (r *eventSourced) Subagents(
	ctx context.Context, chatID string,
) ([]domain.ActivitySubagent, error) {
	return r.store.Subagents(ctx, chatID)
}

func (r *eventSourced) Interruptions(
	ctx context.Context, chatID string,
) ([]domain.ActivityInterruption, error) {
	return r.store.Interruptions(ctx, chatID)
}

func (r *eventSourced) Choices(
	ctx context.Context, chatID string,
) ([]domain.ActivityChoice, error) {
	return r.store.Choices(ctx, chatID)
}

func (r *eventSourced) PendingChoices(
	ctx context.Context, chatID string,
) ([]domain.ActivityChoice, error) {
	return r.store.PendingChoices(ctx, chatID)
}

func (r *eventSourced) RecentToolCalls(
	ctx context.Context, chatIDs []string, since time.Time, limit int,
) ([]domain.ActivityToolCall, error) {
	return r.store.RecentToolCalls(ctx, chatIDs, since, limit)
}

func (r *eventSourced) Payload(_ context.Context, ref string) ([]byte, error) {
	data, err := r.store.Content().Get(ref)
	if err != nil {
		return nil, ErrNotFound
	}
	return data, nil
}

func (r *eventSourced) Forget(ctx context.Context, chatID string) error {
	if err := r.store.DeleteChat(ctx, chatID); err != nil {
		return fmt.Errorf("agentactivity: forget rows: %w", err)
	}
	if err := r.ax.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agentactivity: forget: %w", err)
	}
	return nil
}
