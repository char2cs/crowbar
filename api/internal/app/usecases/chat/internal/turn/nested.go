package turn

import (
	"context"
	"log/slog"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// routeNestedSubagentEvent reports whether ev belongs to one of runner's own
// chat's currently-open SUBAGENTS — a whole second conversation a provider's
// own multi-agent tool call opened (see observation.go's openNestedSubagent)
// — rather than being genuinely foreign. namesAnotherConversation already
// told the two apart from an ordinary top-level event by session id; this is
// the second check that tells a subagent's own child thread apart from an
// actually unrelated conversation, which namesAnotherConversation's own
// session-id comparison alone cannot do (both look identical: a session id
// that is not the runner's current one).
//
// True means ev has been fully handled (routed into the subagent's own
// nested activity or silently dropped as an unsupported kind for it) and the
// caller must not fall through to its own "foreign conversation" drop.
func (t *Turns) routeNestedSubagentEvent(
	ctx context.Context,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) (bool, error) {
	if runner.CurrentChatID == "" || ev.SessionID == "" {
		return false, nil
	}
	open, err := t.activity.IsSubagentOpen(ctx, runner.CurrentChatID, ev.SessionID)
	if err != nil {
		slog.WarnContext(ctx, "agent: nested-session routing: check open subagent", "err", err)
		return false, nil
	}
	if !open {
		return false, nil
	}
	return true, t.handleNestedObservation(ctx, runner.CurrentChatID, ev.SessionID, ev)
}

// handleNestedObservation records ev into subagentID's own nested activity
// instead of chatID's top-level turn — the durable slice of a codex-shaped
// child thread's own turn/item stream this pass implements: its own tool
// calls, and its own final reply text on close. A kind not handled below
// (message_delta live streaming, idle, a permission ask the child itself
// raised, plan updates, reasoning...) is silently dropped, same as an
// unmapped event anywhere else in this package — a live-only signal this
// pass does not yet surface is not an error, and a permission a nested
// conversation raises has nobody positioned to answer it yet regardless.
func (t *Turns) handleNestedObservation(
	ctx context.Context,
	chatID, subagentID string,
	ev engineagents.CanonicalEvent,
) error {
	now := time.Now()
	switch ev.Kind {
	case engineagents.HookTurnStop, engineagents.HookTurnFailed:
		note(ctx, "nested subagent turn closed",
			t.activity.StopSubagent(ctx, chatID, subagentID, "", ev.Message, now))
		t.restateAsyncWork(ctx, chatID)
	case engineagents.HookToolPre:
		note(ctx, "nested subagent tool invoked", t.activity.InvokeSubagentTool(ctx,
			agentactivity.SubagentToolInput{
				ChatID: chatID, SubagentID: subagentID, ToolID: toolID(ev),
				Name: ev.Tool.Name, Target: ev.Tool.Target, Request: ev.Tool.Input, Now: now,
			}))
	case engineagents.HookToolPost, engineagents.HookToolFail:
		note(ctx, "nested subagent tool completed", t.activity.CompleteSubagentTool(ctx,
			agentactivity.SubagentToolResultInput{
				ChatID: chatID, SubagentID: subagentID, ToolID: toolID(ev),
				Name: ev.Tool.Name, Target: ev.Tool.Target, Result: ev.Tool.Result,
				Status: toolStatus(ev), Error: ev.Tool.Error, DurationMS: ev.Tool.DurationMS, Now: now,
			}))
	}
	return nil
}
