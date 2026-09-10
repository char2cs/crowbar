package turn

import (
	"context"
	"log/slog"
	"sync"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
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
	case engineagents.HookPlanUpdate:
		// Live only, restated wholesale — see turns.planUpdate. Nothing durable is
		// written, so a provider mapping this cannot corrupt a transcript either.
		if t.planUpdate != nil && len(ev.Plan) > 0 {
			t.planUpdate(chat.ID, chat.WorkspaceID, ev.Plan)
		}
	case engineagents.HookIdle:
		// ARMS a reconcile; closes nothing. This routinely arrives microseconds
		// BEFORE the turn's own close — see idle.go.
		t.recordIdle(chat)
	case engineagents.HookReasoningDelta:
		// Live only — see recordLiveText. Nothing durable is written, so a provider
		// that maps either of these can never corrupt a transcript with them.
		t.recordLiveText(chat, ev, DeltaKindReasoning)
	case engineagents.HookToolOutputDelta:
		t.recordLiveText(chat, ev, DeltaKindToolOutput)
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
		// Closing the last open tool call after the turn itself already closed is the
		// other half of closeTurnFromStop's OpenWork fallback: open work may now be
		// zero too.
		t.restateAsyncWork(ctx, chat.ID)
	case engineagents.HookSubagentPre:
		note(ctx, "subagent started",
			t.activity.StartSubagent(ctx, chat.ID, subagentID(ev), ev.Subagent.AgentType, now))
	case engineagents.HookSubagentPost:
		note(ctx, "subagent stopped",
			t.activity.StopSubagent(ctx, chat.ID, subagentID(ev), ev.Subagent.AgentType, now))
		t.restateAsyncWork(ctx, chat.ID)
	case engineagents.HookNotification, engineagents.HookPermission,
		engineagents.HookElicitation:
		// Minted ONCE, threaded to both calls below — two independent
		// choiceID/interruptionID draws each mint their own fallbackID()
		// when PromptID is empty (Codex's own mapping never sets one),
		// pairing a choice with an interruption that was never opened. See
		// TestRegression_APermissionWithNoPromptIDStillPairsItsChoiceAndInterruption.
		cid := ""
		if ev.Choice != nil {
			cid = choiceID(ctx, chat.ID, ev.Choice)
		}
		iid := answerdesk.PermissionInterruptionID(cid)
		if iid == "" {
			iid = interruptionID(ctx, chat.ID, ev)
		}
		note(ctx, "interrupted", t.activity.Interrupt(
			ctx, chat.ID, iid, ev.Interrupt.Kind, ev.Interrupt.Detail, now,
		))

		t.openChoice(ctx, chat, runner, agent, ev, cid, raw, now)
	case engineagents.HookCompactPre:
		// Arm BEFORE anything else below: codex's own compact_start round trip
		// (api transport) wraps its contextCompaction item/started..completed in
		// a turn/started..completed pair too, riding the exact wire event
		// turn_stop/turn_failed already consume unconditionally — see
		// compaction.go. A hooks-transport compact_pre (claude has none today;
		// codex's own disconnected companion PTY does) maps no turn_id, so this
		// is a no-op for it.
		t.armCompaction(chat.ID, ev.TurnID)
		// codex reports no trigger at all, so fall back to Crowbar's own
		// record of having just asked for this — never overrides a provider
		// (claude) that DOES report one. See manualCompact.peek's own doc.
		detail := ev.Interrupt.Detail
		if detail == "" && t.manualCompact.peek(chat.ID) {
			detail = "manual"
		}
		note(ctx, "interrupted", t.activity.Interrupt(
			ctx, chat.ID, interruptionID(ctx, chat.ID, ev), ev.Interrupt.Kind, detail, now,
		))
		// Neither ever confirms via a user_prompt hook, so no turn is ever open
		// when this fires — Interrupt's own idle-chat handling (commands/
		// interrupt.go) has ALREADY resolved the record above regardless of
		// whether compact_post goes on to arrive (it does not reliably, over
		// hooks-transport). settleCompactDelivery only settles the pending
		// prompt delivery when THIS compaction is genuinely what was
		// dispatched as one — see its own doc for why that gate is load-
		// bearing, not optional.
		t.settleCompactDelivery(ctx, chat, runner, agent)
		// The ledger record above is already resolved by the time any reader
		// sees it (same idle-chat shortcut), so it can never drive a LIVE
		// "Compacting…" indicator. This direct push is the only signal that
		// can. compact_post is not reliable, so the receiving client is
		// responsible for a bounded self-heal rather than waiting forever —
		// see use-workspace-agent-chats-stream.ts.
		if t.compactionStatus != nil {
			t.compactionStatus(chat.ID, chat.WorkspaceID, true)
		}
	case engineagents.HookCompactPost:
		// SAME fallback as compact_pre, and NOT redundant with it: this event's
		// own ResolveInterruption call REBUILDS the interruption from scratch
		// (see manualCompact.peek's doc) and would otherwise clobber "manual"
		// back to empty moments after compact_pre set it — live-confirmed.
		detail := ev.Interrupt.Detail
		if detail == "" && t.manualCompact.consume(chat.ID) {
			detail = "manual"
		}
		note(ctx, "interruption resolved", t.activity.ResolveInterruption(
			ctx, chat.ID, interruptionID(ctx, chat.ID, ev), ev.Interrupt.Kind, detail, now,
		))
		// Settled already by compact_pre in the ordinary (idle-chat) case —
		// this is the defensive twin for the day compaction happens mid-turn
		// and compact_pre's Interrupt call found the chat genuinely busy.
		t.settleCompactDelivery(ctx, chat, runner, agent)
		if t.compactionStatus != nil {
			t.compactionStatus(chat.ID, chat.WorkspaceID, false)
		}
	}
	return nil
}

func (t *Turns) openChoice(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	id string,
	raw []byte,
	now time.Time,
) {
	if ev.Choice == nil {
		return
	}
	chatID := chat.ID
	// A choice never durably opened in the ledger must never be held for a
	// human or auto-approved: both paths would act on a choice the ledger
	// never recorded, and the provider's own AnswerChoice call would reject
	// it as no longer pending. Falling through here leaves the CLI's own
	// native prompt as the only path, same as holdForAnswer's own silent
	// fallback for every other unanswerable-from-Crowbar reason.
	if err := t.activity.OpenChoice(ctx, agentactivity.ChoiceInput{
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
	}); err != nil {
		note(ctx, "choice opened", err)
		return
	}

	t.holdForAnswer(ctx, chat, runner, agent, ev, id, raw)
}

// choiceID falls back to inflight.RecordID, not fallbackID, when the
// provider gives no PromptID to build promptCorrelationKey from (codex's own
// permission payload never does — see vocabulary.yaml's `permission` entry).
// RecordID keys on the SAME hook delivery id the answer-relay itself
// correlates by, so a redelivered ask (a dropped connection retried, a
// buffered hook replayed) mints the identical choiceID both times instead of
// a fresh one nobody can ever resolve against — the same idempotency
// RecordID already gives every durable turn/message record for the same
// reason (see inflight.RecordID's own doc).
func choiceID(ctx context.Context, chatID string, prompt *engineagents.ChoicePrompt) string {
	if key := promptCorrelationKey(chatID, prompt); key != "" {
		return "choice-" + key
	}
	return "choice-" + inflight.RecordID(ctx)
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

// interruptionID falls back to inflight.RecordID for the same redelivery
// reason choiceID does — see that function's own doc.
//
// Compaction is keyed by ev.TurnID, not just chatID+kind: codex's compact_pre
// (item/started) and compact_post (item/completed) both map turn_id from the
// SAME wrapping turn/started..turn/completed envelope (codex.yaml), so a pair
// still agrees on one id and compact_post still resolves what compact_pre
// opened. Without the turn id, every compaction a chat ever has shared the
// identical row key and each new one silently overwrote the last one's
// transcript position and manual/automatic label — confirmed live. claude's
// own PreCompact/PostCompact map no turn_id at all (claude.yaml), so this
// falls back to the old fixed shape for it, unchanged.
func interruptionID(ctx context.Context, chatID string, ev engineagents.CanonicalEvent) string {
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
	return "interrupt-" + inflight.RecordID(ctx)
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
