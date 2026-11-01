package kit

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Message is one prompt or settled main-agent message a chat's rows say: Seq
// is the row it is shown at, ID the message's own id ("" for a prompt), and
// Provider the provider its turn ran on.
type Message struct {
	Seq                    int64
	ID, TurnID, Role, Text string
	Provider               string
}

// Said is what rows say, in the order a client shows it: each prompt (a
// Crowbar one once its turn took it, where that turn started) and each settled
// message of the main agent where it began. It is a deliberately independent
// reader of the rows, not the production turn fold; the two were checked
// against each other on random row sequences.
func Said(rows []ledger.Row) []Message {
	var out []Message
	var text ledger.MessageFold
	parked := map[string]string{}  // a Crowbar prompt's text, by the turn it is for
	providers := providersOf(rows) // by turn
	for _, r := range rows {
		text.Apply(r)
		if r.Scope != "" {
			continue
		}
		if said, ok := parked[r.TurnID]; ok && r.Kind == ledger.KindTurnStarted {
			out = append(out, Message{Seq: r.Seq, TurnID: r.TurnID, Role: "user", Text: said})
			delete(parked, r.TurnID)
		}
		body, err := ledger.DecodeBody[ledger.UserMessage](r)
		if r.Kind != ledger.KindUserMessage || err != nil {
			continue
		}
		m := Message{Seq: r.Seq, TurnID: r.TurnID, Role: "user", Text: body.Text}
		switch body.Origin {
		case ledger.OriginCrowbar:
			parked[r.TurnID] = body.Text
			continue
		case ledger.OriginHarness:
			m.Role = "harness"
		case ledger.OriginNote:
			m.Provider = "crowbar"
		default:
		}
		out = append(out, m)
	}
	for _, m := range text.Messages() {
		if m.Key.Scope == "" && m.Settled && m.Text != "" {
			out = append(out, Message{Seq: m.FirstSeq, ID: m.Key.MessageID, TurnID: m.Key.TurnID, Role: "assistant", Text: m.Text})
		}
	}
	for i := range out {
		out[i].Provider = cmp.Or(out[i].Provider, providers[out[i].TurnID])
	}
	slices.SortStableFunc(out, func(a, b Message) int { return cmp.Compare(a.Seq, b.Seq) })
	return out
}

// providersOf is the provider each turn ran on: its start's, else that of the
// runner that ended it.
func providersOf(rows []ledger.Row) map[string]string {
	byRunner, byTurn := map[string]string{}, map[string]string{}
	for _, r := range rows {
		switch r.Kind {
		case ledger.KindRunnerStarted:
			if b, err := ledger.DecodeBody[ledger.RunnerStarted](r); err == nil {
				byRunner[b.RunnerID] = b.Provider
			}
		case ledger.KindTurnStarted:
			if b, err := ledger.DecodeBody[ledger.TurnStarted](r); err == nil && byTurn[r.TurnID] == "" {
				byTurn[r.TurnID] = b.Provider
			}
		case ledger.KindTurnEnded:
			if b, err := ledger.DecodeBody[ledger.TurnEnded](r); err == nil && byTurn[r.TurnID] == "" {
				byTurn[r.TurnID] = byRunner[b.RunnerID]
			}
		case ledger.KindUserMessage, ledger.KindPromptSettled, ledger.KindSessionBound, ledger.KindSessionLost,
			ledger.KindMarker, ledger.KindRunnerExited, ledger.KindTurnFailed, ledger.KindAssistantText,
			ledger.KindReasoning, ledger.KindToolCall, ledger.KindAsk, ledger.KindAskResolved,
			ledger.KindSubagent, ledger.KindPlan:
			// names no provider
		}
	}
	return byTurn
}

// Bodies are the bodies of rows of kind, in order; a body that does not
// decode is left out.
func Bodies[B any](rows []ledger.Row, kind ledger.Kind) []B {
	var out []B
	for _, r := range rows {
		var b B
		if r.Kind == kind && json.Unmarshal(r.Body, &b) == nil {
			out = append(out, b)
		}
	}
	return out
}
