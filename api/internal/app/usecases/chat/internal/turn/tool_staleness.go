package turn

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// toolStalenessCeiling is how long a tool call may sit Running with no
// tool_post before a READ gives up waiting for one. It is deliberately the
// same ceiling, and the same dead-letter-net role, as subagentStalenessCeiling
// beside it: generous, because a legitimate tool call runs long, and read-only,
// so the durable event log is never touched and a genuinely late tool_post
// still resolves the SAME row for real when it arrives.
const toolStalenessCeiling = subagentStalenessCeiling

// withStaleToolCallsClosed marks (for this read only) any tool call still
// Running past toolStalenessCeiling as abandoned.
//
// OpenWork already ran its SUBAGENTS through withStaleSubagentsClosed and its
// TOOL CALLS through nothing at all, and that asymmetry is a live wedge: a
// provider that fires tool_pre and never the matching tool_post leaves one row
// Running forever, chat-wide, with no time bound — exactly what
// withStaleSubagentsClosed exists to stop for the other half. Measured on
// claude 2026-09-23 (chat 7745f69f): a ScheduleWakeup tool_pre arrived two
// seconds AFTER the turn's own Stop had already closed it cleanly, relit the
// chat through restateAsyncWork, and no PostToolUse ever followed. claude
// declares not_emitted: [... idle], so the provider-idle fuse cannot fire for
// it, and abandonedMessage is gated on OpenWork — which that one row held true
// forever. The spinner ran until the user pressed Stop.
//
// A row with no StartedAt at all is left alone: an age this cannot measure is
// not an age it may call stale, and reading the zero time as "since year one"
// would abandon every such call on sight.
func withStaleToolCallsClosed(
	calls []domain.ActivityToolCall,
	now time.Time,
) []domain.ActivityToolCall {
	out := make([]domain.ActivityToolCall, len(calls))
	for i, c := range calls {
		if c.Status == domain.ToolStatusRunning && !c.StartedAt.IsZero() &&
			now.Sub(c.StartedAt) > toolStalenessCeiling {
			ended := now
			c.EndedAt = &ended
			c.Status = domain.ToolStatusAbandoned
		}
		out[i] = c
	}
	return out
}
