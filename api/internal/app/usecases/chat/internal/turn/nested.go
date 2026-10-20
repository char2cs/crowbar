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
// caller must not treat it as foreign; ask is what an ask it raised is held on.
func (t *Turns) routeNestedSubagentEvent(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
) (routed bool, ask string, err error) {
	if runner.CurrentChatID == "" || ev.SessionID == "" {
		return false, "", nil
	}
	open, err := t.activity.IsSubagentOpen(ctx, runner.CurrentChatID, ev.SessionID)
	if err != nil {
		slog.WarnContext(ctx, "agent: nested-session routing: check open subagent", "err", err)
		return false, "", nil
	}
	if !open {
		return false, "", nil
	}
	ask, err = t.handleNestedObservation(ctx, runner, agent, ev.SessionID, ev, raw)
	return true, ask, err
}

// handleNestedObservation records ev into subagentID's own nested activity
// instead of chatID's top-level turn — the durable slice of a child
// conversation's own stream: its own tool calls, its own final reply text on
// close, and any ask the person must answer, which goes through the same path
// as a top-level ask. Other kinds (live deltas, idle, plan, reasoning...) are a
// filter: a child conversation's live stream is not part of the chat's record.
func (t *Turns) handleNestedObservation(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	subagentID string,
	ev engineagents.CanonicalEvent,
	raw []byte,
) (string, error) {
	chatID := runner.CurrentChatID
	now := time.Now()
	switch ev.Kind {
	case engineagents.HookPermission, engineagents.HookElicitation:
		chat, err := t.chatForRunner(ctx, runner)
		if err != nil {
			return "", err
		}
		return t.raiseAsk(ctx, chat, runner, agent, ev, raw, now)
	case engineagents.HookTurnStop, engineagents.HookTurnFailed:
		return "", wrote("nested subagent turn closed",
			t.activity.StopSubagent(ctx, chatID, subagentID, "", ev.Message, now))
	case engineagents.HookToolPre:
		return "", wrote("nested subagent tool invoked", t.activity.InvokeSubagentTool(ctx,
			agentactivity.SubagentToolInput{
				ChatID: chatID, SubagentID: subagentID, ToolID: toolID(ev),
				Name: ev.Tool.Name, Kind: ev.Tool.Kind, Locations: toolLocations(ev),
				Target: ev.Tool.Target, Request: ev.Tool.Input, Now: now,
			}))
	case engineagents.HookToolPost, engineagents.HookToolFail:
		return "", wrote("nested subagent tool completed", t.activity.CompleteSubagentTool(ctx,
			agentactivity.SubagentToolResultInput{
				ChatID: chatID, SubagentID: subagentID, ToolID: toolID(ev),
				Name: ev.Tool.Name, Kind: ev.Tool.Kind, Locations: toolLocations(ev),
				Target: ev.Tool.Target, Result: ev.Tool.Result,
				Status: toolStatus(ev), Error: ev.Tool.Error, DurationMS: ev.Tool.DurationMS, Now: now,
			}))
	}
	return "", nil
}

// routeSubagentTool drops a tool call the payload attributes to a subagent: only
// the main agent's own tool calls are part of this chat's log.
func routeSubagentTool(ev engineagents.CanonicalEvent) bool {
	toolEvent := ev.Kind == engineagents.HookToolPre || ev.Kind == engineagents.HookToolPost ||
		ev.Kind == engineagents.HookToolFail
	return toolEvent && ev.Subagent != nil
}
