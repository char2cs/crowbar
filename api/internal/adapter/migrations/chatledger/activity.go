package chatledger

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// The legacy tool statuses that ended a call well.
const (
	toolOK      = "ok"
	toolRunning = "running"
)

// openTurnPrefix is the id the old daemon gave a reply still open on a runner:
// "open-<chat id>-<runner id>".
const openTurnPrefix = "open-"

// turnOf is the ledger turn a legacy record's turn id names: the turn holding
// that legacy row, or for an open reply's id the runner's turn at at.
func (c *conversion) turnOf(legacyTurnID string, at time.Time) *group {
	if g, ok := c.byID[legacyTurnID]; ok && legacyTurnID != "" {
		return g
	}
	runnerID, ok := strings.CutPrefix(legacyTurnID, openTurnPrefix+c.chatID+"-")
	if !ok || runnerID == "" {
		return nil
	}
	var found *group
	for _, g := range c.groups {
		if g.runnerID != runnerID && firstRunner(g.replies) != runnerID {
			continue
		}
		if found == nil || !g.head.StartedAt.After(at) {
			found = g
		}
	}
	return found
}

// subagents restate each subagent as a start under its own scope and a stop:
// its last reply when it ended, interrupted at its turn's end when it never did.
func (c *conversion) subagents(agents []LegacySubagent) {
	c.agents = map[string]*group{}
	for _, a := range agents {
		g := c.turnOf(a.TurnID, a.StartedAt)
		if g == nil || a.ID == "" {
			c.skipf("subagent %s: no turn of the chat holds it", a.ID)
			continue
		}
		c.agents[a.ID] = g
		start := latest(a.StartedAt, g.start)
		c.emit(ruleSubagent, start, a.Seq, ledger.KindSubagent, g.id, a.ID,
			ledger.SubagentBody{Phase: ledger.SubagentStart, Type: a.AgentType})
		stop := ledger.SubagentBody{Phase: ledger.SubagentStop, Result: c.lastMessage(a)}
		at := latest(g.end, start)
		if a.EndedAt != nil {
			at = latest(*a.EndedAt, start)
		} else {
			stop.Status = string(ledger.SubagentInterrupted)
		}
		c.emit(ruleSubagent, at, a.Seq, ledger.KindSubagent, g.id, a.ID, stop)
	}
}

func (c *conversion) lastMessage(a LegacySubagent) string {
	if a.Messages == "" || a.Messages == "null" {
		return ""
	}
	var messages []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(a.Messages), &messages); err != nil {
		c.skipf("subagent %s: messages unreadable: %v", a.ID, err)
		return ""
	}
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Text
}

// tools restate each call as its invocation and, once it ended, its outcome,
// with the blob refs kept as they are. A call no turn holds is skipped.
func (c *conversion) tools(tools []LegacyTool) {
	for _, t := range tools {
		g := c.turnOf(t.TurnID, t.StartedAt)
		if g == nil {
			g = c.agents[t.SubagentID]
		}
		if g == nil || t.ID == "" {
			c.skipf("tool %s: no turn of the chat holds it", t.ID)
			continue
		}
		pre := ledger.ToolCall{
			ToolID: t.ID, Phase: ledger.ToolPre, Name: t.Name, Kind: t.Kind, Target: t.Target,
			Locations: c.locations(t), InputRef: t.RequestRef,
		}
		start := latest(t.StartedAt, g.start)
		c.emit(ruleTool, start, t.Seq, ledger.KindToolCall, g.id, t.SubagentID, pre)
		if t.EndedAt == nil {
			continue
		}
		end := ledger.ToolCall{
			ToolID: t.ID, Phase: ledger.ToolFail, Name: t.Name, Kind: t.Kind, ResultRef: t.ResultRef,
			Status: t.Status, Error: t.Error, DurationMS: t.DurationMS, Diff: t.Diff,
		}
		if t.Status == toolOK || t.Status == "" || t.Status == toolRunning {
			end.Phase = ledger.ToolPost
		}
		c.emit(ruleTool, latest(*t.EndedAt, start), t.Seq, ledger.KindToolCall, g.id, t.SubagentID, end)
	}
}

func (c *conversion) locations(t LegacyTool) []ledger.ToolLocation {
	if t.Locations == "" || t.Locations == "null" {
		return nil
	}
	var out []ledger.ToolLocation
	if err := json.Unmarshal([]byte(t.Locations), &out); err != nil {
		c.skipf("tool %s: locations unreadable: %v", t.ID, err)
		return nil
	}
	return out
}
