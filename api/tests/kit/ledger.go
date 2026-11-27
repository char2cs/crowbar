package kit

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

type Message struct {
	Seq                    int64
	ID, TurnID, Role, Text string
	Provider               string
}

func Said(rows []ledger.Row) []Message {
	var out []Message
	var text ledger.MessageFold
	parked := map[string]string{}
	providers := providersOf(rows)
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
			ledger.KindSubagent, ledger.KindPlan, ledger.KindMove, ledger.KindShell:
		}
	}
	return byTurn
}

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
