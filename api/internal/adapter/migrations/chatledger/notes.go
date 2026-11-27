package chatledger

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

const (
	kindPermission  = "permission"
	kindElicitation = "elicitation"
	kindCompaction  = "compaction"
	choiceElicit    = "elicitation"
)

var markerKinds = []string{
	ledger.MarkerStopped, ledger.MarkerProviderSwitched, ledger.MarkerModelChanged,
	ledger.MarkerEffortChanged, ledger.MarkerNotification,
}

func (c *conversion) interruptions(interruptions []LegacyInterruption) {
	for _, in := range interruptions {
		g := c.turnOf(in.TurnID, in.At)
		var turnID string
		at, end := in.At, in.At
		if g != nil {
			turnID, at, end = g.id, latest(in.At, g.start), g.end
		}
		switch {
		case in.Kind == kindCompaction:
			c.emit(ruleMarker, at, in.Seq, ledger.KindMarker, turnID, "", ledger.Marker{Kind: ledger.MarkerCompactionStarted})
			if in.ResolvedAt != nil {
				end = *in.ResolvedAt
			}
			c.emit(ruleMarker, latest(end, at), in.Seq, ledger.KindMarker, turnID, "", ledger.Marker{Kind: ledger.MarkerCompactionEnded})
		case slices.Contains(markerKinds, in.Kind):
			c.emit(ruleMarker, at, in.Seq, ledger.KindMarker, turnID, "", ledger.Marker{Kind: in.Kind, Detail: in.Detail})
		case in.Kind == kindInferred:
		case in.Kind == kindPermission || in.Kind == kindElicitation:
		default:
			c.skipf("interruption %s: unknown kind %q", in.ID, in.Kind)
		}
	}
}

func (c *conversion) choices(choices []LegacyChoice) {
	for _, ch := range choices {
		if ch.ID == "" {
			c.skipf("choice: no id")
			continue
		}
		g := c.turnOf(ch.TurnID, ch.At)
		var turnID string
		at, end := ch.At, ch.At
		if g != nil {
			turnID, at, end = g.id, latest(ch.At, g.start), g.end
		}
		event := kindPermission
		if ch.Kind == choiceElicit {
			event = kindElicitation
		}
		c.emit(ruleAsk, at, ch.Seq, ledger.KindAsk, turnID, "", ledger.Ask{
			AskID: ch.ID, Event: event, Kind: ch.Kind, PromptID: ch.PromptID, ToolID: ch.ToolID, ToolName: ch.ToolName,
			Title: ch.Title, Question: ch.Question, Mode: ch.Mode, Multi: ch.Multi,
			Options: c.rawJSON(ch.ID, "options", ch.Options), Questions: c.rawJSON(ch.ID, "questions", ch.Questions),
			Schema: c.rawJSON(ch.ID, "schema", ch.Schema),
		})
		resolved := ledger.AskResolved{AskID: ch.ID, Resolution: resolution(ch)}
		if ch.ResolvedAt != nil {
			end = *ch.ResolvedAt
			if err := json.Unmarshal([]byte(ch.AnsweredOptionIDs), &resolved.OptionIDs); ch.AnsweredOptionIDs != "" && err != nil {
				c.skipf("choice %s: answered options unreadable: %v", ch.ID, err)
			}
		}
		c.emit(ruleAsk, latest(end, at), ch.Seq, ledger.KindAskResolved, turnID, "", resolved)
	}
}

func resolution(ch LegacyChoice) string {
	known := []string{ledger.ResolutionAnswered, ledger.ResolutionProceeded, ledger.ResolutionAbandoned}
	if ch.ResolvedAt == nil || !slices.Contains(known, ch.Resolution) {
		return ledger.ResolutionAbandoned
	}
	return ch.Resolution
}

func (c *conversion) rawJSON(choiceID, column, s string) json.RawMessage {
	if s == "" || s == "null" {
		return nil
	}
	if !json.Valid([]byte(s)) {
		c.skipf("choice %s: %s unreadable", choiceID, column)
		return nil
	}
	return json.RawMessage(s)
}

var promptSettles = map[string]string{
	"dispatching": ledger.ReasonDaemonRestart,
	"spawned":     ledger.ReasonDaemonRestart,
	"uncertain":   ledger.ReasonDaemonRestart,
	"failed":      ledger.ReasonDeliveryFailed,
	"settled":     ledger.ReasonNotAcknowledged,
}

const promptAccepted = "accepted"

func (c *conversion) prompts(prompts []LegacyPrompt) {
	for _, p := range prompts {
		if g, ok := c.byID[p.RequestID]; ok && g.head.ID == p.RequestID {
			continue
		}
		reason, ok := promptSettles[p.State]
		if !ok {
			why := "unknown state " + p.State
			if p.State == promptAccepted {
				why = "accepted, and no turn holds it"
			}
			c.skipf("prompt %s: %s", p.RequestID, why)
			continue
		}
		id := turnID(c.chatID, p.RequestID)
		c.emit(rulePrompt, p.CreatedAt, 0, ledger.KindUserMessage, id, "",
			ledger.UserMessage{RequestID: p.RequestID, Text: p.Text, Origin: ledger.OriginCrowbar})
		c.emit(rulePrompt, latest(p.UpdatedAt, p.CreatedAt), 0, ledger.KindPromptSettled, "", "",
			ledger.PromptSettled{RequestID: p.RequestID, Reason: reason})
	}
}

func (c *conversion) runners(runners []LegacyRunner) {
	c.adopted = map[string]bool{}
	last := c.latest
	for _, r := range runners {
		at := latest(last, r.StartedAt)
		if at.IsZero() {
			at = time.Unix(0, 0)
		}
		c.emit(ruleRunner, at, 0, ledger.KindRunnerStarted, "", "",
			ledger.RunnerStarted{RunnerID: r.ID, Provider: r.ProviderID})
		c.adopted[r.ID] = true
	}
}
