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

// handleObservation records what the agent is DOING, as distinct from what it
// said.
//
// Every failure here is logged and swallowed. These events are the difference
// between a legible surface and an opaque one, but none of them is the
// conversation: losing a tool record is a gap in a timeline, while failing the
// hook would break the vendor CLI's turn.
func (u *Usecase) handleObservation(
	ctx context.Context,
	runner domain.AgentRunner,
	ev engineagents.CanonicalEvent,
) error {
	chat, ok, err := u.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}
	now := time.Now()

	switch ev.Kind {
	case engineagents.HookToolPre:
		u.note(ctx, "tool invoked", u.activity.InvokeTool(ctx, agentactivity.ToolInput{
			ChatID: chat.ID, ToolID: toolID(ev), Name: ev.Tool.Name, Target: ev.Tool.Target,
			Request: ev.Tool.Input, Now: now,
		}))
	case engineagents.HookToolPost:
		u.note(ctx, "tool completed", u.activity.CompleteTool(ctx, agentactivity.ToolResultInput{
			ChatID: chat.ID, ToolID: toolID(ev), Name: ev.Tool.Name, Target: ev.Tool.Target,
			Result: ev.Tool.Result, Status: toolStatus(ev), DurationMS: ev.Tool.DurationMS, Now: now,
		}))
	case engineagents.HookSubagentPre:
		u.note(ctx, "subagent started",
			u.activity.StartSubagent(ctx, chat.ID, subagentID(ev), ev.Subagent.AgentType, now))
	case engineagents.HookSubagentPost:
		u.note(ctx, "subagent stopped",
			u.activity.StopSubagent(ctx, chat.ID, subagentID(ev), ev.Subagent.AgentType, now))
	case engineagents.HookNotification, engineagents.HookPermission, engineagents.HookCompactPre:
		u.note(ctx, "interrupted", u.activity.Interrupt(
			ctx, chat.ID, interruptionID(chat.ID, ev), ev.Interrupt.Kind, ev.Interrupt.Detail, now,
		))
	case engineagents.HookCompactPost:
		u.note(ctx, "interruption resolved", u.activity.ResolveInterruption(
			ctx, chat.ID, interruptionID(chat.ID, ev), ev.Interrupt.Kind, ev.Interrupt.Detail, now,
		))
	}
	return nil
}

func (u *Usecase) note(ctx context.Context, what string, err error) {
	if err != nil {
		slog.WarnContext(ctx, "agent: observation: "+what, "err", err)
	}
}

// toolID falls back to the delivery id when a provider does not supply one.
// Without a stable id a completion cannot be matched to its invocation, and the
// call would render as two unrelated rows.
func toolID(ev engineagents.CanonicalEvent) string {
	if ev.Tool != nil && ev.Tool.ID != "" {
		return ev.Tool.ID
	}
	return "tool-" + fallbackID()
}

func toolStatus(ev engineagents.CanonicalEvent) string {
	if ev.Tool == nil || ev.Tool.Status == "" {
		return domain.ToolStatusOK
	}
	return ev.Tool.Status
}

func subagentID(ev engineagents.CanonicalEvent) string {
	if ev.Subagent != nil && ev.Subagent.ID != "" {
		return ev.Subagent.ID
	}
	// Subagent starts and stops do not balance on either provider — they observe
	// different populations — so an anonymous one gets its own id rather than
	// being folded onto a sibling's.
	return "subagent-" + fallbackID()
}

// interruptionID keys an interruption by its KIND within a chat, so a compaction
// beginning and ending are the same record rather than two.
//
// Providers do not give these events ids. Keying on kind is the only pairing
// available, and it is correct for the events that pair at all: a compaction
// cannot overlap another compaction in one session.
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

// fallbackID is a monotonic per-process id for events a provider left anonymous.
// It is deliberately not random: two of these landing in the same nanosecond must
// still be distinguishable, and a counter guarantees that where a clock does not.
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

// handleTelemetry records the provider's own report of context, cost, rate limits
// and resolved model.
//
// It is CURRENT STATE, not history: thousands of "19% used" observations exist
// only to be superseded, so it never enters the event log. It is held in memory
// per chat and served alongside the chat, which is also why losing it costs
// nothing — the next report is moments away.
func (u *Usecase) handleTelemetry(
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
		// A provider that reported nothing must not overwrite what it reported a
		// moment ago. Claude's usage block is null until the first turn completes,
		// so an early report is genuinely empty rather than a reset.
		return nil
	}
	u.telemetry.set(chat.ID, report)
	return nil
}

// Telemetry returns the latest report for a chat, and whether there is one.
func (u *Usecase) Telemetry(chatID string) (engineagents.Telemetry, bool) {
	return u.telemetry.get(chatID)
}

// telemetryStore holds the most recent report per chat.
//
// In memory, deliberately. Telemetry describes a LIVE provider process; a report
// that outlived the process it describes would be a confident stale number, and
// showing one is worse than showing none.
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

// ChatActivity is what an agent did during a chat, as distinct from what it said.
type ChatActivity struct {
	ToolCalls     []domain.ActivityToolCall
	Subagents     []domain.ActivitySubagent
	Interruptions []domain.ActivityInterruption
}

// maxActivityPage bounds one activity read. A long chat holds thousands of tool
// calls and a client asking for "everything" would render none of them usefully.
const maxActivityPage = 500

// ReadActivity returns a chat's tool calls, subagents and interruptions.
//
// Tool calls page (they are the unbounded one); subagents and interruptions do
// not, because a turn produces a handful of each and paging three cursors to draw
// one timeline is a worse contract than reading all of two of them.
func (u *Usecase) ReadActivity(
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
	return ChatActivity{ToolCalls: calls, Subagents: subagents, Interruptions: interruptions}, nil
}

// ReadToolPayload resolves one tool call's request or result.
//
// The ref is looked up from the chat's OWN tool calls rather than accepted from
// the caller: a content ref is a global address, and taking one from a client
// would let any chat read any other chat's payloads.
func (u *Usecase) ReadToolPayload(
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
