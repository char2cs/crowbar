package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (u *turnUsecase) handleObservation(
	ctx context.Context,
	runner domain.AgentRunner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
) error {
	chat, ok, err := u.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}
	now := time.Now()

	switch ev.Kind {
	case engineagents.HookMessageDelta:

		u.recordMessageDelta(ctx, chat, runner, ev)
	case engineagents.HookToolPre:
		note(ctx, "tool invoked", u.activity.InvokeTool(ctx, agentactivity.ToolInput{
			ChatID: chat.ID, ToolID: toolID(ev), Name: ev.Tool.Name, Target: ev.Tool.Target,
			Request: ev.Tool.Input, Now: now,
		}))
	case engineagents.HookToolPost, engineagents.HookToolFail:

		note(ctx, "tool completed", u.activity.CompleteTool(ctx, agentactivity.ToolResultInput{
			ChatID: chat.ID, ToolID: toolID(ev), Name: ev.Tool.Name, Target: ev.Tool.Target,
			Result: ev.Tool.Result, Status: toolStatus(ev), Error: ev.Tool.Error,
			DurationMS: ev.Tool.DurationMS, Now: now,
		}))
	case engineagents.HookSubagentPre:
		note(ctx, "subagent started",
			u.activity.StartSubagent(ctx, chat.ID, subagentID(ev), ev.Subagent.AgentType, now))
	case engineagents.HookSubagentPost:
		note(ctx, "subagent stopped",
			u.activity.StopSubagent(ctx, chat.ID, subagentID(ev), ev.Subagent.AgentType, now))
	case engineagents.HookNotification, engineagents.HookPermission,
		engineagents.HookElicitation, engineagents.HookCompactPre:
		note(ctx, "interrupted", u.activity.Interrupt(
			ctx, chat.ID, interruptionID(chat.ID, ev), ev.Interrupt.Kind, ev.Interrupt.Detail, now,
		))

		u.openChoice(ctx, chat, runner, agent, ev, raw, now)
	case engineagents.HookCompactPost:
		note(ctx, "interruption resolved", u.activity.ResolveInterruption(
			ctx, chat.ID, interruptionID(chat.ID, ev), ev.Interrupt.Kind, ev.Interrupt.Detail, now,
		))
	}
	return nil
}

func (u *turnUsecase) openChoice(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
	now time.Time,
) {
	if ev.Choice == nil {
		return
	}
	chatID := chat.ID
	id := choiceID(chatID, ev.Choice)
	note(ctx, "choice opened", u.activity.OpenChoice(ctx, agentactivity.ChoiceInput{
		ChatID:   chatID,
		ChoiceID: id,
		Kind:     ev.Choice.Kind,
		PromptID: ev.Choice.PromptID,
		ToolName: ev.Choice.ToolName,
		Title:    ev.Choice.Title,
		Question: ev.Choice.Question,
		Mode:     ev.Choice.Mode,
		Multi:    ev.Choice.Multi,
		Options:  choiceOptions(ev.Choice.Options),

		Questions: choiceQuestions(ev.Choice.Questions),
		Schema:    string(ev.Choice.Schema),
		Now:       now,
	}))

	u.answers.holdForAnswer(ctx, chat, runner, agent, ev, id, raw)
}

func choiceID(chatID string, prompt *engineagents.ChoicePrompt) string {
	if prompt.PromptID == "" {
		return "choice-" + fallbackID()
	}
	if prompt.ToolName == "" {
		return "choice-" + chatID + "-" + prompt.PromptID
	}
	return "choice-" + chatID + "-" + prompt.PromptID + "-" + prompt.ToolName
}

func choiceQuestions(in []engineagents.PromptQuestion) []domain.ActivityChoiceQuestion {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.ActivityChoiceQuestion, 0, len(in))
	for _, q := range in {
		out = append(out, domain.ActivityChoiceQuestion{
			ID: q.ID, Title: q.Title, Text: q.Text, Multi: q.Multi,
			Options: choiceOptions(q.Options),
		})
	}
	return out
}

func choiceOptions(in []engineagents.ChoiceOption) []domain.ActivityChoiceOption {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.ActivityChoiceOption, 0, len(in))
	for _, o := range in {
		out = append(out, domain.ActivityChoiceOption{
			ID: o.ID, Kind: o.Kind, Label: o.Label, Description: o.Description,
		})
	}
	return out
}

func note(
	ctx context.Context,
	what string,
	err error,
) {
	if err == nil {
		return
	}
	slog.WarnContext(ctx, "agent: observation: "+what, "err", err)
}

func toolID(ev engineagents.CanonicalEvent) string {
	if ev.Tool != nil && ev.Tool.ID != "" {
		return ev.Tool.ID
	}
	return "tool-" + fallbackID()
}

func toolStatus(ev engineagents.CanonicalEvent) string {
	if ev.Kind == engineagents.HookToolFail {
		return domain.ToolStatusError
	}
	if ev.Tool == nil || ev.Tool.Status == "" {
		return domain.ToolStatusOK
	}
	return ev.Tool.Status
}

func subagentID(ev engineagents.CanonicalEvent) string {
	if ev.Subagent != nil && ev.Subagent.ID != "" {
		return ev.Subagent.ID
	}

	return "subagent-" + fallbackID()
}

func interruptionID(chatID string, ev engineagents.CanonicalEvent) string {
	kind := ""
	if ev.Interrupt != nil {
		kind = ev.Interrupt.Kind
	}
	if kind == engineagents.InterruptCompaction {
		return "interrupt-" + chatID + "-" + kind
	}
	return "interrupt-" + fallbackID()
}

var (
	fallbackMu  sync.Mutex
	fallbackSeq int64
)

func fallbackID() string {
	fallbackMu.Lock()
	defer fallbackMu.Unlock()
	fallbackSeq++
	return time.Now().UTC().Format("20060102T150405.000000000") + "-" + itoa(fallbackSeq)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func (u *turnUsecase) handleTelemetry(
	ctx context.Context,
	runner domain.AgentRunner,
	agent engineagents.Agent,
	raw []byte,
) error {
	chat, ok, err := u.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}
	report, err := agent.ParseTelemetry(raw, time.Now())
	if err != nil {
		slog.DebugContext(ctx, "agent: telemetry: parse", "err", err, "provider", runner.ProviderID)
		return nil
	}
	if report.Empty() {
		return nil
	}
	u.telemetry.set(chat.ID, report)
	return nil
}

func (u *turnUsecase) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return u.telemetry.get(chatID)
}

type telemetryStore struct {
	mu      sync.RWMutex
	reports map[string]engineagents.Telemetry
}

func newTelemetryStore() *telemetryStore {
	return &telemetryStore{reports: map[string]engineagents.Telemetry{}}
}

func (s *telemetryStore) set(chatID string, report engineagents.Telemetry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[chatID] = report
}

func (s *telemetryStore) get(chatID string) (engineagents.Telemetry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.reports[chatID]
	return report, ok
}

func (s *telemetryStore) forget(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.reports, chatID)
}

type ChatActivity struct {
	ToolCalls     []domain.ActivityToolCall
	Subagents     []domain.ActivitySubagent
	Interruptions []domain.ActivityInterruption

	Choices []domain.ActivityChoice
}

const maxActivityPage = 500

func (u *turnUsecase) ReadActivity(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) (ChatActivity, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: chat: %w", err)
	}
	if limit <= 0 || limit > maxActivityPage {
		limit = maxActivityPage
	}
	calls, err := u.activity.ToolCalls(ctx, chatID, after, limit)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: tool calls: %w", err)
	}
	subagents, err := u.activity.Subagents(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: subagents: %w", err)
	}
	interruptions, err := u.activity.Interruptions(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: interruptions: %w", err)
	}
	choices, err := u.activity.Choices(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: choices: %w", err)
	}
	return ChatActivity{
		ToolCalls:     calls,
		Subagents:     subagents,
		Interruptions: interruptions,
		Choices:       choices,
	}, nil
}

func (u *turnUsecase) ReadPendingChoices(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read pending choices: chat: %w", err)
	}
	choices, err := u.activity.PendingChoices(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: read pending choices: %w", err)
	}
	return choices, nil
}

func (u *turnUsecase) ReadToolPayload(
	ctx context.Context,
	chatID, toolID, side string,
) ([]byte, error) {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read tool payload: chat: %w", err)
	}
	calls, err := u.activity.ToolCalls(ctx, chatID, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("agent: read tool payload: tool calls: %w", err)
	}
	for _, c := range calls {
		if c.ID != toolID {
			continue
		}
		ref := c.RequestRef
		if side == "result" {
			ref = c.ResultRef
		}
		if ref == "" {
			return nil, agentactivity.ErrNotFound
		}
		return u.activity.Payload(ctx, ref)
	}
	return nil, agentactivity.ErrNotFound
}
