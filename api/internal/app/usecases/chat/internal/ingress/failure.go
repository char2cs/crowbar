package ingress

import (
	"cmp"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func mapFailure(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, Effects, error) {
	if rows, ok, err := compactionEnd(st, ev.TurnID); ok || err != nil {
		return rows, Effects{}, err
	}
	fail := ledger.TurnFailed{Reason: ledger.ReasonTurnFailed, Detail: ev.Message}
	reply := ""
	if ev.Failure != nil {
		fail.Reason = cmp.Or(ev.Failure.Reason, fail.Reason)
		fail.Detail = cmp.Or(ev.Failure.Detail, ev.Message)
		fail.Kind = ev.Failure.Kind
		if ev.Failure.Detail != "" {
			reply = ev.Message // the provider's explanation is the detail; this is its reply
		}
	}
	failed, closes, fresh, err := ledger.FailTurnOf(st, src.RunnerID, ev.TurnID, fail)
	if err != nil {
		return nil, Effects{}, err
	}
	if fresh {
		// Nothing ran: the prompts the runner carried were never taken, as with a notice.
		settled, err := ledger.SettleCarried(st, src.RunnerID, false, ledger.ReasonTurnFailed)
		return append([]ledger.Row{failed}, settled...), Effects{}, err
	}
	rows, err := closingRows(st, failed.TurnID, reply, src, false)
	if err != nil {
		return nil, Effects{}, err
	}
	rows = append(rows, failed)
	if closes {
		settled, err := settleSteer(st, src.RunnerID, ledger.ReasonTurnFailed)
		if err != nil {
			return nil, Effects{}, err
		}
		rows = append(rows, settled...)
	}
	return rows, Effects{Ended: failed.TurnID}, nil
}

// noticeRows fail the turn runnerID runs for what its screen says, or with
// none running, record the notice as a failure of its own and settle the
// prompts it carries unconsumed.
func noticeRows(st ledger.State, runnerID string, notice engineagents.TerminalNotice, live Unrecorded) ([]ledger.Row, error) {
	fail := ledger.TurnFailed{Reason: notice.Kind, Detail: notice.Text}
	if notice.Kind == engineagents.TerminalNoticeUsageLimit {
		fail.Kind = engineagents.FailureLimit
	}
	if _, running := st.RunningTurn(); !running {
		failed, _, _, err := ledger.FailTurnOf(st, runnerID, uuid.NewString(), fail)
		if err != nil {
			return nil, err
		}
		settled, err := ledger.SettleCarried(st, runnerID, false, notice.Kind)
		return append([]ledger.Row{failed}, settled...), err
	}
	failed, closes, _, err := ledger.FailTurnOf(st, runnerID, "", fail)
	if err != nil || !closes {
		return nil, err
	}
	rows, err := closingRows(st, failed.TurnID, "", Source{Live: live}, false)
	if err != nil {
		return nil, err
	}
	settled, err := settleSteer(st, runnerID, ledger.ReasonTurnFailed)
	return append(append(rows, failed), settled...), err
}
