package ingress

import (
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func pendingSteer(st ledger.State, runnerID string) (ledger.Prompt, bool) {
	for _, p := range st.PendingPrompts() {
		if p.Delivery == ledger.DeliverySteer && p.RunnerID == runnerID {
			return p, true
		}
	}
	return ledger.Prompt{}, false
}

func settleSteer(st ledger.State, runnerID, reason string) ([]ledger.Row, error) {
	return ledger.Settle(st, func(p ledger.Prompt) bool {
		return p.Delivery == ledger.DeliverySteer && p.RunnerID == runnerID
	}, false, reason)
}

func steerRows(st ledger.State, src Source) ([]ledger.Row, []byte, error) {
	p, ok := pendingSteer(st, src.RunnerID)
	if !ok {
		return nil, nil, nil
	}
	var stdout []byte
	deliverable := false
	if src.Steer != nil {
		stdout, deliverable = src.Steer(p)
	}
	if !src.Held || !deliverable || !startsOn(st, src.RunnerID) {
		settled, err := settleSteer(st, src.RunnerID, ledger.ReasonDeliveryFailed)
		return settled, nil, err
	}
	started, err := ledger.NewRow(ledger.KindTurnStarted, p.TurnID, "", ledger.TurnStarted{
		RequestID: p.RequestID, RunnerID: src.RunnerID, Provider: src.Provider,
		SessionID: src.SessionID, Persists: src.Persists,
	})
	if err != nil {
		return nil, nil, err
	}
	return []ledger.Row{started}, stdout, nil
}
