// Package agentactivity is the event-sourced repository for a chat's
// conversation record: its turns, tool calls, subagents and interruptions.
//
// It is a SEPARATE aggregate from agentchat, and deliberately so. agentchat holds
// what the sidebar reads on every frame — title, placement, whether the chat is
// working — a handful of events per chat. This holds hundreds of events per turn.
// Joining them would put a sidebar repaint behind a tool-call storm, and would
// make one aggregate's snapshot cost the other's write rate.
//
// The two are joined by chat id and neither writes to the other. That is not a
// style preference: a torn write across two aggregates with no transaction is
// what previously bricked a chat.
package agentactivity

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// maxOCCAttempts bounds optimistic-concurrency retries. Concurrent hooks for one
// chat can version-collide, so the losers retry; Send re-reads the current version
// each attempt, so a retry converges.
const maxOCCAttempts = 8

// TurnInput records one complete turn.
type TurnInput struct {
	ChatID     string
	TurnID     string
	Role       string
	ProviderID string
	RunnerID   string
	SessionID  string
	Text       string
	Effort     string
	Now        time.Time
}

// ToolInput records a tool invocation. Request is the raw payload; it goes to the
// content store and never into the event log.
type ToolInput struct {
	ChatID  string
	ToolID  string
	Name    string
	Target  string
	Request []byte
	Now     time.Time
}

// ToolResultInput records a tool completing, successfully or not.
type ToolResultInput struct {
	ChatID string
	ToolID string
	Name   string
	Target string
	Result []byte
	Status string
	// Error is what the provider said went wrong. It is truncated to a MESSAGE
	// before it reaches the aggregate — the full text is in Result, which goes to
	// the content store — because a tool that fails on a hundred kilobytes of
	// compiler output must not put a hundred kilobytes into every later snapshot.
	Error      string
	DurationMS int
	Now        time.Time
}

// ChoiceInput records the agent asking a human to decide something.
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
	Schema   string
	Now      time.Time
}

// maxToolErrorBytes bounds the inline copy of a tool failure. It is a caption for
// a timeline row, not the failure itself.
const maxToolErrorBytes = 2 << 10

// EventStore is the conversation record's repository.
type EventStore interface {
	// AppendTurn records a turn that is complete on arrival — a user prompt, or
	// an assistant reply with no open turn to close.
	AppendTurn(ctx context.Context, in TurnInput) error

	// OpenTurn starts the assistant turn that tool calls attach to.
	OpenTurn(ctx context.Context, in TurnInput) error

	// CloseTurn completes the open assistant turn with its hook-confirmed text.
	CloseTurn(ctx context.Context, in TurnInput) error

	// Abandon closes whatever a chat left open without inventing a reply. It is
	// the reconcile path for a CLI that died mid-turn.
	Abandon(ctx context.Context, chatID string, now time.Time) error

	InvokeTool(ctx context.Context, in ToolInput) error
	CompleteTool(ctx context.Context, in ToolResultInput) error

	StartSubagent(ctx context.Context, chatID, subagentID, agentType string, now time.Time) error
	StopSubagent(ctx context.Context, chatID, subagentID, agentType string, now time.Time) error

	Interrupt(ctx context.Context, chatID, id, kind, detail string, now time.Time) error
	ResolveInterruption(ctx context.Context, chatID, id, kind, detail string, now time.Time) error

	// OpenChoice records a prompt the agent is blocked on until a human answers.
	OpenChoice(ctx context.Context, in ChoiceInput) error

	// ResolveChoice records a prompt no longer waiting on anybody.
	//
	// It is the EXPLICIT channel, and the one an answer will arrive down. It is not
	// the only one, and deliberately not the main one: a permission answered at the
	// PTY reports nothing at all, so a completed tool call and a closed turn resolve
	// their prompts on their own. A prompt that never cleared would strand the UI on
	// a question nobody is asking.
	ResolveChoice(ctx context.Context, chatID, choiceID, resolution string, now time.Time) error

	// AnswerChoice records a human DECIDING a prompt through Crowbar, naming the
	// option ids they picked. It refuses an answer to a prompt that is no longer
	// pending, and one that names an option the prompt never offered — checks
	// nothing downstream could make, because by then the decision has already been
	// rendered into a provider's own JSON.
	AnswerChoice(ctx context.Context, chatID, choiceID string, optionIDs []string, now time.Time) error

	// Turns returns a chat's turns in sequence order. limit 0 means all.
	Turns(ctx context.Context, chatID string, after, before int64, limit int) ([]domain.ActivityTurn, error)
	// TurnsBefore returns the newest turns below a sequence, ascending. It is the
	// backward page: reading descending with a limit is what keeps a ten-thousand
	// turn chat from loading its whole history to show the last twenty.
	TurnsBefore(ctx context.Context, chatID string, before int64, limit int) ([]domain.ActivityTurn, error)
	TurnsSince(ctx context.Context, chatID string, cut time.Time) ([]domain.ActivityTurn, error)
	CountTurns(ctx context.Context, chatID string) (int64, error)

	LastTurnAt(ctx context.Context, chatID, providerID string) (time.Time, bool, error)
	LastTurnForSession(ctx context.Context, chatID, providerID, sessionID string) (time.Time, bool, error)
	HasTurnAtOrAfter(ctx context.Context, chatID, providerID string, since time.Time) (bool, error)

	ToolCalls(ctx context.Context, chatID string, after int64, limit int) ([]domain.ActivityToolCall, error)
	Subagents(ctx context.Context, chatID string) ([]domain.ActivitySubagent, error)
	Interruptions(ctx context.Context, chatID string) ([]domain.ActivityInterruption, error)
	// Choices returns a chat's prompts, pending and resolved alike.
	Choices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)
	// PendingChoices returns only the prompts still waiting on a human.
	PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)
	RecentToolCalls(ctx context.Context, chatIDs []string, since time.Time, limit int) ([]domain.ActivityToolCall, error)

	// Payload resolves a content ref. Returns ErrNotFound when retention has
	// swept it, which is an ordinary outcome rather than a failure.
	Payload(ctx context.Context, ref string) ([]byte, error)

	// Forget purges a chat's record: the aggregate and its projected rows.
	Forget(ctx context.Context, chatID string) error
}

type eventSourced struct {
	ax    asynx.Asynx[domain.AgentActivity]
	store *store.Store
}

// NewEventSourced builds the repository over the activity event log and its own
// read model, with the content store rooted at contentRoot.
func NewEventSourced(
	ax asynx.Asynx[domain.AgentActivity],
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

// send dispatches a command with OCC retry, ASYNCHRONOUSLY.
//
// Validation is never retried (the command is wrong, not late), a full queue is
// backpressure rather than a version race, and only a pipeline collision is worth
// another attempt.
func (r *eventSourced) send(ctx context.Context, cmd asynxModels.Command[domain.AgentActivity]) error {
	return r.dispatch(ctx, r.ax.Send, cmd)
}

// sendWait dispatches and BLOCKS until the projection has folded.
//
// The turn commands take this path and the activity commands do not, and the
// split is not arbitrary. A turn is read back immediately by three callers — the
// chat surface renders it, the handoff assembler builds a document from it, and
// the durable-prompt recovery decides from it whether a message was already
// delivered. That last one is why an async write here is a correctness bug rather
// than a latency one: a recovery that reads a lagging model concludes the prompt
// never landed and dispatches it a second time.
//
// Tool, subagent and interruption events have no such reader and are hundreds of
// times more frequent, so they stay async: a barrier on that path would put every
// tool call behind a projection fold.
func (r *eventSourced) sendWait(ctx context.Context, cmd asynxModels.Command[domain.AgentActivity]) error {
	return r.dispatch(ctx, r.ax.SendWait, cmd)
}

type sendFunc func(
	ctx context.Context,
	cmd asynxModels.Command[domain.AgentActivity],
) (asynxModels.Event[domain.AgentActivity], error)

func (r *eventSourced) dispatch(
	ctx context.Context,
	send sendFunc,
	cmd asynxModels.Command[domain.AgentActivity],
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
		Text: in.Text, Effort: in.Effort, Now: in.Now,
	})
}

func (r *eventSourced) Abandon(ctx context.Context, chatID string, now time.Time) error {
	return r.sendWait(ctx, commands.Abandon{ChatID: chatID, Now: now})
}

func (r *eventSourced) InvokeTool(ctx context.Context, in ToolInput) error {
	ref, err := r.store.Content().Put(in.Request)
	if err != nil {
		// A payload that could not be stored must not cost the record of the call
		// itself: the tool DID run, and showing it with no arguments is far better
		// than not showing it.
		ref = ""
	}
	return r.send(ctx, commands.InvokeTool{
		ChatID: in.ChatID, ToolID: in.ToolID, Name: in.Name, Target: in.Target,
		RequestRef: ref, Now: in.Now,
	})
}

func (r *eventSourced) CompleteTool(ctx context.Context, in ToolResultInput) error {
	ref, err := r.store.Content().Put(in.Result)
	if err != nil {
		ref = ""
	}
	return r.send(ctx, commands.CompleteTool{
		ChatID: in.ChatID, ToolID: in.ToolID, Name: in.Name, Target: in.Target,
		ResultRef: ref, Status: in.Status, Error: truncate(in.Error, maxToolErrorBytes),
		DurationMS: in.DurationMS, Now: in.Now,
	})
}

// truncate cuts a caption to size on a rune boundary, so a multi-byte character
// straddling the limit is dropped whole rather than left as a broken one.
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

func (r *eventSourced) OpenChoice(ctx context.Context, in ChoiceInput) error {
	return r.send(ctx, commands.OpenChoice{
		ChatID: in.ChatID, ChoiceID: in.ChoiceID, Kind: in.Kind,
		PromptID: in.PromptID, ToolName: in.ToolName,
		Title: in.Title, Question: in.Question, Mode: in.Mode, Multi: in.Multi,
		Options: in.Options, Schema: in.Schema, Now: in.Now,
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
	ctx context.Context, chatID, choiceID string, optionIDs []string, now time.Time,
) error {
	return r.send(ctx, commands.AnswerChoice{
		ChatID: chatID, ChoiceID: choiceID, OptionIDs: optionIDs, Now: now,
	})
}

func (r *eventSourced) StartSubagent(
	ctx context.Context, chatID, subagentID, agentType string, now time.Time,
) error {
	return r.send(ctx, commands.StartSubagent{
		ChatID: chatID, SubagentID: subagentID, AgentType: agentType, Now: now,
	})
}

func (r *eventSourced) StopSubagent(
	ctx context.Context, chatID, subagentID, agentType string, now time.Time,
) error {
	return r.send(ctx, commands.StopSubagent{
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
	// Rows first: Forget fires OnForget, but a hard delete of the aggregate must
	// not leave the conversation readable if that handler fails.
	if err := r.store.DeleteChat(ctx, chatID); err != nil {
		return fmt.Errorf("agentactivity: forget rows: %w", err)
	}
	if err := r.ax.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agentactivity: forget: %w", err)
	}
	return nil
}
