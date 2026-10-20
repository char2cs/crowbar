package turn

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// handleObservation records what happened inside a turn, and returns the ask
// a permission or elicitation was held on, if one was. A write that failed is
// its error: the hook is refused, never acknowledged with nothing written.
func (t *Turns) handleObservation(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
) (string, error) {
	chat, err := t.chatForRunner(ctx, runner)
	if err != nil {
		return "", err
	}
	now := time.Now()

	switch ev.Kind {
	case engineagents.HookMessageDelta:
		return "", t.recordMessageDelta(ctx, chat, runner, ev)
	case engineagents.HookPlanUpdate:
		return "", t.recordPlan(ctx, chat, runner, ev, now)
	case engineagents.HookIdle:
		return "", t.recordIdle(ctx, chat.ID, runner)
	case engineagents.HookReasoningDelta:
		// Live only — see recordLiveText. Nothing durable is written, so a provider
		// that maps either of these can never corrupt a transcript with them.
		t.recordLiveText(chat, ev, DeltaKindReasoning)
	case engineagents.HookToolOutputDelta:
		t.recordLiveText(chat, ev, DeltaKindToolOutput)
	case engineagents.HookDiffUpdate:
		t.recordLiveReplacement(chat, ev, DeltaKindDiff)
	case engineagents.HookToolPre, engineagents.HookToolPost, engineagents.HookToolFail:
		return "", t.recordTool(ctx, chat.ID, ev, now)
	case engineagents.HookSubagentPre, engineagents.HookSubagentPost:
		return "", t.recordSubagent(ctx, chat.ID, ev, now)
	case engineagents.HookNotification, engineagents.HookPermission,
		engineagents.HookElicitation:
		return t.raiseAsk(ctx, chat, runner, agent, ev, raw, now)
	case engineagents.HookCompactPre:
		return "", t.recordCompactionStart(ctx, chat, runner, agent, ev, now)
	case engineagents.HookCompactPost:
		return "", t.recordCompactionEnd(ctx, chat, runner, agent, ev, now)
	}
	return "", nil
}

// recordPlan pushes the restated plan live and keeps it on the open turn, so it
// survives the turn closing, a restart and a paged read.
func (t *Turns) recordPlan(
	ctx context.Context, chat domain.Chat, runner engineagents.Runner, ev engineagents.CanonicalEvent, now time.Time,
) error {
	if t.feed.Plan != nil && len(ev.Plan) > 0 {
		t.feed.Plan(chat.ID, chat.WorkspaceID, ev.Plan)
	}
	steps := make([]domain.ActivityPlanStep, 0, len(ev.Plan))
	for _, step := range ev.Plan {
		steps = append(steps, domain.ActivityPlanStep{Text: step.Text, Status: step.Status})
	}
	return wrote("plan updated", t.activity.UpdatePlan(ctx, agentactivity.PlanInput{
		ChatID: chat.ID, ProviderID: runner.ProviderID, RunnerID: runner.ID,
		SessionID: t.boundSession(ctx, chat.ID, runner.ProviderID), Steps: steps, Now: now,
	}))
}

func (t *Turns) recordTool(ctx context.Context, chatID string, ev engineagents.CanonicalEvent, now time.Time) error {
	if ev.Kind == engineagents.HookToolPre {
		return wrote("tool invoked", t.activity.InvokeTool(ctx, agentactivity.ToolInput{
			ChatID: chatID, ToolID: toolID(ev), Name: ev.Tool.Name, Kind: ev.Tool.Kind,
			Locations: toolLocations(ev), Target: ev.Tool.Target,
			Request: ev.Tool.Input, Diff: toolStartDiff(ev.Tool),
			Now: now,
		}))
	}
	if err := t.activity.CompleteTool(ctx, agentactivity.ToolResultInput{
		ChatID: chatID, ToolID: toolID(ev), Name: ev.Tool.Name, Kind: ev.Tool.Kind,
		Locations: toolLocations(ev), Target: ev.Tool.Target,
		Result: ev.Tool.Result, Diff: toolResultDiff(ev.Tool),
		Status: toolStatus(ev), Error: ev.Tool.Error,
		DurationMS: ev.Tool.DurationMS, Now: now,
	}); err != nil {
		return wrote("tool completed", err)
	}
	return t.openNestedSubagent(ctx, chatID, ev, now)
}

func (t *Turns) recordSubagent(ctx context.Context, chatID string, ev engineagents.CanonicalEvent, now time.Time) error {
	if ev.Subagent == nil || ev.Subagent.ID == "" {
		return nil
	}
	if ev.Kind == engineagents.HookSubagentPre {
		return wrote("subagent started",
			t.activity.StartSubagent(ctx, chatID, subagentID(ev), ev.Subagent.AgentType, now))
	}
	return wrote("subagent stopped",
		t.activity.StopSubagent(ctx, chatID, subagentID(ev), ev.Subagent.AgentType, "", now))
}

func toolLocations(ev engineagents.CanonicalEvent) []domain.ActivityToolLocation {
	if ev.Tool == nil || len(ev.Tool.Locations) == 0 {
		return nil
	}
	out := make([]domain.ActivityToolLocation, 0, len(ev.Tool.Locations))
	for _, location := range ev.Tool.Locations {
		out = append(out, domain.ActivityToolLocation{Path: location.Path, Line: location.Line})
	}
	return out
}

// choiceID is the id a choice is opened and its ask held under: derived from
// the provider's own prompt id when it gives one, fresh otherwise.
func choiceID(chatID string, prompt *engineagents.ChoicePrompt) string {
	if key := promptCorrelationKey(chatID, prompt); key != "" {
		return "choice-" + key
	}
	return "choice-" + uuid.NewString()
}

// promptCorrelationKey is the "chatID-promptID-toolName" suffix choiceID and
// a permission's interruptionID both derive from, so a permission choice's
// paired interruption is a prefix swap away, never a second lookup.
func promptCorrelationKey(chatID string, prompt *engineagents.ChoicePrompt) string {
	if prompt == nil || prompt.PromptID == "" {
		return ""
	}
	if prompt.ToolName == "" {
		return chatID + "-" + prompt.PromptID
	}
	return chatID + "-" + prompt.PromptID + "-" + prompt.ToolName
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

// wrote is err, the failed write of what a hook recorded, with its context.
func wrote(what string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("agent: hook: %s: %w", what, err)
}

func toolID(ev engineagents.CanonicalEvent) string {
	if ev.Tool != nil && ev.Tool.ID != "" {
		return ev.Tool.ID
	}
	return "tool-" + fallbackID()
}

func toolStatus(ev engineagents.CanonicalEvent) string {
	if ev.Kind == engineagents.HookToolFail {
		if ev.Tool != nil && ev.Tool.Status == domain.ToolStatusDeclined {
			return domain.ToolStatusDeclined
		}
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

// openNestedSubagent starts tracking ev.Tool.NestedSessionID as one of chatID's
// own subagents, the first time a tool completion names an id this chat has
// never recorded — see the mapping's own doc on the field for what populates it
// and why (a provider's own multi-agent tool call reporting the thread id of the
// agent it spawned or is addressing). A no-op for every ordinary tool call,
// which maps no such field.
//
// EVER RECORDED, not "open right now": a provider names the same child on every
// call it makes about it, including the one that completes because the child
// finished. Reopening it would read a stopped child as live work forever
// (TestRegression_ANestedSubagentIsNeverReopenedAfterItStopped).
//
// OpenNestedSubagent, not StartSubagent: Go never learns why a tool call
// named this id, only that one did, but StartSubagent's own ensureTurn is
// wrong here regardless of provider vocabulary — see OpenNestedSubagent's
// own doc for why a tool-triggered open must never touch the top-level turn.
func (t *Turns) openNestedSubagent(
	ctx context.Context,
	chatID string,
	ev engineagents.CanonicalEvent,
	now time.Time,
) error {
	if ev.Tool == nil || ev.Tool.NestedSessionID == "" {
		return nil
	}
	known, err := t.activity.Subagents(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: hook: nested subagent: check already recorded: %w", err)
	}
	for _, s := range known {
		if s.ID == ev.Tool.NestedSessionID {
			return nil
		}
	}
	return wrote("nested subagent opened",
		t.activity.OpenNestedSubagent(ctx, chatID, ev.Tool.NestedSessionID, now))
}

// interruptionID is the id an interruption is opened under. A compaction is
// keyed by its turn id when the descriptor maps one, so compact_pre and
// compact_post agree on one row and a later compaction never overwrites it.
func interruptionID(chatID string, ev engineagents.CanonicalEvent) string {
	kind := ""
	if ev.Interrupt != nil {
		kind = ev.Interrupt.Kind
	}
	if kind == engineagents.InterruptCompaction {
		if ev.TurnID != "" {
			return "interrupt-" + chatID + "-" + kind + "-" + ev.TurnID
		}
		return "interrupt-" + chatID + "-" + kind
	}
	if kind == engineagents.InterruptPermission {
		if key := promptCorrelationKey(chatID, ev.Choice); key != "" {
			return "interrupt-" + key
		}
	}
	return "interrupt-" + uuid.NewString()
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

// holdForAnswer parks this prompt on the answer desk under choiceID, so
// Crowbar's UI can decide it for the person, and returns the ask its sender's
// answer is held on.
//
// It is silent — "" and no error — for every reason a prompt may be
// unanswerable from Crowbar: a provider that declares no answer format for the
// event, or a payload too large to hold, is left to the CLI's own UI.
func (t *Turns) holdForAnswer(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	choiceID string,
	raw []byte,
) string {
	if choiceID == "" {
		return ""
	}
	// Handed over to the provider's own view: holding the relay keeps the CLI
	// from drawing the prompt on the surface the user is looking at, and the
	// card that answers it renders only on the chat surface.
	if t.runners.ShowingNativeView(runner.ID) {
		return ""
	}
	capability, answerable := agent.AnswerCapability(ev.Kind)
	if !answerable {
		return ""
	}
	if len(raw) > answerdesk.MaxPayloadBytes {
		slog.DebugContext(ctx, "agent: answer: prompt payload too large to answer",
			"chat_id", chat.ID, "event", ev.Kind, "bytes", len(raw))
		return ""
	}
	t.answers.Hold(answerdesk.Prompt{
		ChoiceID: choiceID,
		ChatID:   chat.ID,
		RunnerID: runner.ID,
		Event:    ev.Kind,
		Raw:      append([]byte(nil), raw...),
		Keys:     capability,
	})
	return choiceID
}
