package turn

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// subagentStalenessCeiling is how long a subagent may sit open with no
// subagent_post before Crowbar gives up waiting for one and treats it as
// abandoned for READ purposes only. Generous on purpose — a legitimate
// multi-agent delegation can run long — this is a dead-letter net, not a
// normal timeout, and it never touches the durable event log: a genuinely
// late subagent_post still resolves the SAME entry for real the moment it
// arrives (StopSubagent addresses it by id regardless of what a read
// reported in the meantime).
const subagentStalenessCeiling = 2 * time.Hour

// withStaleSubagentsClosed marks (for this read only) any subagent open
// longer than subagentStalenessCeiling as ended.
//
// abandonRunningSubagents (activity's own projection) already closes a
// still-open subagent when its WHOLE chat is given up on — but only then:
// a subagent is deliberately allowed to keep running past its own turn's
// ordinary close (codex hands off and ends its top-level turn right there,
// see TestRegression_CodexTurnStopWithOpenSubagent_KeepsChatWorking), so
// nothing may close one just because a turn ended. That leaves a real gap:
// a subagent_post that is simply lost — dropped, or a background fork whose
// own stop hook never fires — while the ENCLOSING turn finishes completely
// normally (a real reply delivered, nothing stalled, nothing abandoned)
// has no cleanup path at all. Both the chat and the subagent shelf spin
// forever. Caught live: a chat's own turn had genuinely finished, yet one
// forked subagent's clock kept climbing for hours.
func withStaleSubagentsClosed(
	subagents []domain.ActivitySubagent,
	now time.Time,
) []domain.ActivitySubagent {
	out := make([]domain.ActivitySubagent, len(subagents))
	for i, s := range subagents {
		if s.EndedAt == nil && now.Sub(s.StartedAt) > subagentStalenessCeiling {
			ended := now
			s.EndedAt = &ended
		}
		out[i] = s
	}
	return out
}
