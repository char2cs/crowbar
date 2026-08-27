package turn

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (t *Turns) handleObservation(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
) error {
	chat, ok, err := t.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}
	now := time.Now()

	switch ev.Kind {
	case engineagents.HookMessageDelta:

		t.recordMessageDelta(ctx, chat, runner, ev)
	case engineagents.HookToolPre:
		note(ctx, "tool invoked", t.activity.InvokeTool(ctx, agentactivity.ToolInput{
			ChatID: chat.ID, ToolID: toolID(ev), Name: ev.Tool.Name, Target: ev.Tool.Target,
			Request: ev.Tool.Input, Now: now,
		}))
	case engineagents.HookToolPost, engineagents.HookToolFail:

		note(ctx, "tool completed", t.activity.CompleteTool(ctx, agentactivity.ToolResultInput{
			ChatID: chat.ID, ToolID: toolID(ev), Name: ev.Tool.Name, Target: ev.Tool.Target,
			Result: ev.Tool.Result, Status: toolStatus(ev), Error: ev.Tool.Error,
			DurationMS: ev.Tool.DurationMS, Now: now,
		}))
	case engineagents.HookSubagentPre:
		note(ctx, "subagent started",
			t.activity.StartSubagent(ctx, chat.ID, subagentID(ev), ev.Subagent.AgentType, now))
	case engineagents.HookSubagentPost:
		note(ctx, "subagent stopped",
			t.activity.StopSubagent(ctx, chat.ID, subagentID(ev), ev.Subagent.AgentType, now))
	case engineagents.HookNotification, engineagents.HookPermission,
		engineagents.HookElicitation, engineagents.HookCompactPre:
		note(ctx, "interrupted", t.activity.Interrupt(
			ctx, chat.ID, interruptionID(chat.ID, ev), ev.Interrupt.Kind, ev.Interrupt.Detail, now,
		))

		t.openChoice(ctx, chat, runner, agent, ev, raw, now)
	case engineagents.HookCompactPost:
		note(ctx, "interruption resolved", t.activity.ResolveInterruption(
			ctx, chat.ID, interruptionID(chat.ID, ev), ev.Interrupt.Kind, ev.Interrupt.Detail, now,
		))
	}
	return nil
}

func (t *Turns) openChoice(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
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
	note(ctx, "choice opened", t.activity.OpenChoice(ctx, agentactivity.ChoiceInput{
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

	t.holdForAnswer(ctx, chat, runner, agent, ev, id, raw)
	t.autoApproveIfPolicy(ctx, chatID, id, ev, agent)
}

// autoApproveIfPolicy resolves a just-opened choice immediately when the
// chat's permission level clears the prompt's risk tier, using the exact
// render-and-resolve sequence a human's Allow click uses — so even under
// full-auto, the transcript shows a decision was made, not silence.
//
// Only a plain tool-permission prompt (Kind == ChoiceToolPermission) is ever
// eligible: a question prompt (AskUserQuestion) has no Allow option to pick,
// and an elicitation is out of scope by design (see the spec) — both carry no
// meaningful Risk tier and must fall through to the human hold untouched.
func (t *Turns) autoApproveIfPolicy(
	ctx context.Context,
	chatID string,
	choiceID string,
	ev engineagents.CanonicalEvent,
	agent engineagents.Agent,
) {
	if ev.Choice == nil || ev.Choice.Kind != engineagents.ChoiceToolPermission {
		return
	}
	level := t.permissionLevels.Get(chatID)
	if !permission.AutoApprove(level, ev.Choice.Risk) {
		return
	}
	slot, held := t.answers.ByChoiceID(choiceID)
	if !held {
		return
	}
	// Same safety property the human path enforces in AnswerChoice
	// (chat/answers.go): a provider that cannot express "allow" for this
	// event must never have one manufactured for it.
	if !slot.Keys.Accepts(domain.ChoiceOptionAllow) {
		return
	}
	decision := engineagents.AnswerDecision{Key: domain.ChoiceOptionAllow}
	stdout, err := agent.RenderAnswer(slot.Event, slot.Raw, decision)
	if err != nil {
		slog.WarnContext(ctx, "agent: permission: auto-approve: render", "err", err, "choice_id", choiceID)
		return
	}
	if err := t.activity.AnswerChoice(
		ctx, chatID, choiceID, []string{domain.ChoiceOptionAllow}, time.Now(),
	); err != nil {
		slog.WarnContext(ctx, "agent: permission: auto-approve: ledger", "err", err, "choice_id", choiceID)
		return
	}
	t.answers.Resolve(slot, stdout)
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

func (t *Turns) handleTelemetry(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	raw []byte,
) error {
	chat, ok, err := t.chatForRunner(ctx, runner)
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
	t.telemetry.Set(chat.ID, report)
	return nil
}

func (t *Turns) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return t.telemetry.Get(chatID)
}

type ChatActivity struct {
	ToolCalls     []domain.ActivityToolCall
	Subagents     []domain.ActivitySubagent
	Interruptions []domain.ActivityInterruption

	Choices []domain.ActivityChoice
}

const maxActivityPage = 500

func (t *Turns) ReadActivity(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) (ChatActivity, error) {
	if _, err := t.chats.GetChat(ctx, chatID); err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: chat: %w", err)
	}
	if limit <= 0 || limit > maxActivityPage {
		limit = maxActivityPage
	}
	var calls []domain.ActivityToolCall
	var err error
	if after > 0 {
		calls, err = t.activity.ToolCalls(ctx, chatID, after, limit)
	} else {
		calls, err = t.activity.ToolCallsBefore(ctx, chatID, 0, limit)
	}
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: tool calls: %w", err)
	}
	subagents, err := t.activity.Subagents(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: subagents: %w", err)
	}
	interruptions, err := t.activity.Interruptions(ctx, chatID)
	if err != nil {
		return ChatActivity{}, fmt.Errorf("agent: read activity: interruptions: %w", err)
	}
	choices, err := t.activity.Choices(ctx, chatID)
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

func (t *Turns) ReadPendingChoices(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	if _, err := t.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read pending choices: chat: %w", err)
	}
	choices, err := t.activity.PendingChoices(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: read pending choices: %w", err)
	}
	return choices, nil
}

func (t *Turns) ReadToolPayload(
	ctx context.Context,
	chatID, toolID, side string,
) ([]byte, error) {
	if _, err := t.chats.GetChat(ctx, chatID); err != nil {
		return nil, fmt.Errorf("agent: read tool payload: chat: %w", err)
	}
	calls, err := t.activity.ToolCalls(ctx, chatID, 0, 0)
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
		return t.activity.Payload(ctx, ref)
	}
	return nil, agentactivity.ErrNotFound
}

// holdForAnswer parks the hook relay carrying this prompt on the answer desk, so
// Crowbar's UI can decide it for the person instead of leaving them to the
// provider's own terminal prompt.
//
// It is silent — not an error — for every reason a prompt may be unanswerable
// from Crowbar: an un-journalled ingress has no relay to park, a provider that
// declares no answer format for the event cannot be answered at all, and a
// payload too large to hold is left to the provider. In each case the CLI's own
// UI still works.
func (t *Turns) holdForAnswer(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	choiceID string,
	raw []byte,
) {
	deliveryID := inflight.DeliveryID(ctx)
	if deliveryID == "" || choiceID == "" {
		return
	}
	capability, answerable := agent.AnswerCapability(ev.Kind)
	if !answerable {
		return
	}
	if len(raw) > answerdesk.MaxPayloadBytes {
		slog.DebugContext(ctx, "agent: answer: prompt payload too large to answer",
			"chat_id", chat.ID, "event", ev.Kind, "bytes", len(raw))
		return
	}
	t.answers.Hold(deliveryID, answerdesk.Prompt{
		ChoiceID: choiceID,
		ChatID:   chat.ID,
		RunnerID: runner.ID,
		Event:    ev.Kind,
		Raw:      append([]byte(nil), raw...),
		Keys:     capability,
	})
}
