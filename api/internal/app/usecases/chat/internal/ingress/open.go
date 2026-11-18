package ingress

import (
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// openProviderTurn is the turn the provider started as providerTurnID, unless
// none may start on src's runner or it was already recorded.
func openProviderTurn(st ledger.State, providerTurnID string, src Source) ([]ledger.Row, error) {
	if !src.Held || !startsOn(st, src.RunnerID) {
		return nil, nil
	}
	started := ledger.TurnStarted{
		ProviderTurnID: providerTurnID, RunnerID: src.RunnerID, Provider: src.Provider,
		SessionID: src.SessionID, Persists: src.Persists,
	}
	if _, known := st.ProviderTurn(providerTurnID); known {
		// A CLI still inside its last turn's end names that turn on a prompt typed meanwhile.
		// Its words are the proof: a retry's are the ones that turn already took.
		if _, running := st.RunningTurn(); running || !awaitsWords(st, src) {
			return nil, nil
		}
		started.ProviderTurnID = ""
	}
	rows, err := ledger.EndTurn(st, ledger.TurnStatusDone, ledger.ReasonSuperseded)
	if err != nil {
		return nil, err
	}
	opened, err := openRows(st, started, src)
	return append(rows, opened...), err
}

// openRows start the turn the prompt opens: on the oldest pending Crowbar
// prompt its runner carries when the words are that prompt's, or when the
// provider echoed none. A steer is never one: only its carrier's turn end opens its turn.
func openRows(st ledger.State, started ledger.TurnStarted, src Source) ([]ledger.Row, error) {
	wordless := src.Prompt.Text == ""
	switch src.Prompt.Origin {
	case "":
		return bareTurn(started)
	case ledger.OriginProvider:
		if p, ok := pendingFor(st, src); ok {
			started.RequestID = p.RequestID
			row, err := ledger.NewRow(ledger.KindTurnStarted, p.TurnID, "", started)
			return []ledger.Row{row}, err
		}
		if wordless {
			return bareTurn(started)
		}
		overtaken, err := ledger.SettleOvertaken(st, src.RunnerID)
		if err != nil {
			return nil, err
		}
		opened, err := providerPromptTurn(started, src)
		return append(overtaken, opened...), err
	}
	return providerPromptTurn(started, src)
}

// pendingFor is the oldest pending Crowbar prompt src's runner carries that the
// words are, or that the provider echoed none of. A steer is never one.
func pendingFor(st ledger.State, src Source) (ledger.Prompt, bool) {
	for _, p := range st.PendingPrompts() {
		if p.Delivery == ledger.DeliverySteer || (src.Prompt.Text != "" && p.Text != src.Prompt.Text) ||
			(p.RunnerID != "" && p.RunnerID != src.RunnerID) {
			continue
		}
		return p, true
	}
	return ledger.Prompt{}, false
}

// awaitsWords reports whether a prompt Crowbar typed is waiting for exactly src's words.
func awaitsWords(st ledger.State, src Source) bool {
	if src.Prompt.Origin != ledger.OriginProvider || src.Prompt.Text == "" {
		return false
	}
	_, ok := pendingFor(st, src)
	return ok
}

// providerPromptTurn is a turn on words no Crowbar prompt sent: their own
// user_message, then the turn.
func providerPromptTurn(started ledger.TurnStarted, src Source) ([]ledger.Row, error) {
	turnID := uuid.NewString()
	msg, err := ledger.NewRow(ledger.KindUserMessage, turnID, "",
		ledger.UserMessage{Text: src.Prompt.Text, Origin: src.Prompt.Origin})
	if err != nil {
		return nil, err
	}
	row, err := ledger.NewRow(ledger.KindTurnStarted, turnID, "", started)
	return []ledger.Row{msg, row}, err
}

func bareTurn(started ledger.TurnStarted) ([]ledger.Row, error) {
	row, err := ledger.NewRow(ledger.KindTurnStarted, uuid.NewString(), "", started)
	return []ledger.Row{row}, err
}
