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
	Now        time.Time
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

	AnswerChoice(ctx context.Context, chatID, choiceID string, optionIDs []string, now time.Time) error

	Turns(ctx context.Context, chatID string, after, before int64, limit int) ([]domain.ActivityTurn, error)

	TurnsBefore(ctx context.Context, chatID string, before int64, limit int) ([]domain.ActivityTurn, error)
	TurnsSince(ctx context.Context, chatID string, cut time.Time) ([]domain.ActivityTurn, error)
	CountTurns(ctx context.Context, chatID string) (int64, error)

	LastTurnAt(ctx context.Context, chatID, providerID string) (time.Time, bool, error)
	LastTurnForSession(ctx context.Context, chatID, providerID, sessionID string) (time.Time, bool, error)
	HasTurnAtOrAfter(ctx context.Context, chatID, providerID string, since time.Time) (bool, error)

	ToolCalls(ctx context.Context, chatID string, after int64, limit int) ([]domain.ActivityToolCall, error)
	Subagents(ctx context.Context, chatID string) ([]domain.ActivitySubagent, error)
	Interruptions(ctx context.Context, chatID string) ([]domain.ActivityInterruption, error)

	Choices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)

	PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)
	RecentToolCalls(ctx context.Context, chatIDs []string, since time.Time, limit int) ([]domain.ActivityToolCall, error)

	Payload(ctx context.Context, ref string) ([]byte, error)

	Forget(ctx context.Context, chatID string) error
}

type eventSourced struct {
	ax    asynx.Asynx[domain.AgentActivity]
	store *store.Store
}

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

func (r *eventSourced) send(ctx context.Context, cmd asynxModels.Command[domain.AgentActivity]) error {
	return r.dispatch(ctx, r.ax.Send, cmd)
}

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

	if err := r.store.DeleteChat(ctx, chatID); err != nil {
		return fmt.Errorf("agentactivity: forget rows: %w", err)
	}
	if err := r.ax.Forget(ctx, chatID); err != nil {
		return fmt.Errorf("agentactivity: forget: %w", err)
	}
	return nil
}
