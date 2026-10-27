package chatledger

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// The legacy interruption kinds an ask restates, and the one a compaction is.
const (
	kindPermission  = "permission"
	kindElicitation = "elicitation"
	kindCompaction  = "compaction"
	choiceElicit    = "elicitation"
)

// markerKinds are the legacy interruptions that are a marker of the same kind.
var markerKinds = []string{
	ledger.MarkerStopped, ledger.MarkerProviderSwitched, ledger.MarkerModelChanged,
	ledger.MarkerEffortChanged, ledger.MarkerNotification,
}

// interruptions restate the dividers the transcript showed. An ask's own
// interruption is its choice's, and an inferred one is its turn's end.
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
			// inferEnds made it its turn's end, or named why not.
		case in.Kind == kindPermission || in.Kind == kindElicitation:
			c.skipf("interruption %s: %s, restated by its choice", in.ID, in.Kind)
		default:
			c.skipf("interruption %s: unknown kind %q", in.ID, in.Kind)
		}
	}
}

// choices restate each ask as one nothing can answer any more, resolved as it
// was, or abandoned when it never was.
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
			Options: rawJSON(ch.Options), Questions: rawJSON(ch.Questions), Schema: rawJSON(ch.Schema),
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

func rawJSON(s string) json.RawMessage {
	if s == "" || s == "null" || !json.Valid([]byte(s)) {
		return nil
	}
	return json.RawMessage(s)
}

// Why each journal state that never became a turn is settled; accepted is a turn.
var promptSettles = map[string]string{
	"dispatching": ledger.ReasonDaemonRestart,
	"spawned":     ledger.ReasonDaemonRestart,
	"uncertain":   ledger.ReasonDaemonRestart,
	"failed":      ledger.ReasonDeliveryFailed,
	"settled":     ledger.ReasonNotAcknowledged,
}

const promptAccepted = "accepted"

// prompts restate each journalled prompt no turn covers as a prompt settled
// unconsumed: it is shown, and never delivered again.
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

// runners leave the chat live on each CLI the old daemon never saw exit, after
// everything else, so the boot reconcile ends it as it would any other.
func (c *conversion) runners(runners []LegacyRunner) {
	c.adopted = map[string]bool{}
	for _, r := range runners {
		c.emit(ruleRunner, c.lastAt(), 0, ledger.KindRunnerStarted, "", "",
			ledger.RunnerStarted{RunnerID: r.ID, Provider: r.ProviderID})
		c.adopted[r.ID] = true
	}
}

func (c *conversion) lastAt() time.Time {
	if c.latest.IsZero() {
		return time.Unix(0, 0)
	}
	return c.latest
}
