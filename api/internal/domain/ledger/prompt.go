package ledger

import (
	"cmp"
	"time"
)

const (
	OriginCrowbar  = "crowbar"
	OriginProvider = "provider"
	OriginHarness  = "harness"
	OriginNote     = "note"
)

const (
	DeliveryRestart = "restart"
	DeliverySteer   = "steer"
	DeliveryAPI     = "api"
	DeliveryType    = "type"
)

const (
	ReasonProviderBuiltin = "provider_builtin"
	ReasonNotAcknowledged = "not_acknowledged"
	ReasonRunnerExited    = "runner_exited"
	ReasonDaemonRestart   = "daemon_restart"
	ReasonDeliveryFailed  = "delivery_failed"
	ReasonSessionLost     = "session_lost"
	ReasonRetracted       = "retracted"
)

type UserMessage struct {
	RequestID string `json:"requestId,omitempty"`
	Text      string `json:"text"`
	Origin    string `json:"origin"`
	Delivery  string `json:"delivery,omitempty"`
	RunnerID  string `json:"runnerId,omitempty"`
	Dispatch  string `json:"dispatch,omitempty"`
	RetryOf   string `json:"retryOf,omitempty"`
}

type TurnStarted struct {
	RequestID      string `json:"requestId,omitempty"`
	ProviderTurnID string `json:"providerTurnId,omitempty"`
	RunnerID       string `json:"runnerId,omitempty"`
	Provider       string `json:"provider,omitempty"`
	SessionID      string `json:"sessionId,omitempty"`
	Persists       string `json:"persists,omitempty"`
}

type PromptSettled struct {
	RequestID string `json:"requestId"`
	Consumed  bool   `json:"consumed"`
	Reason    string `json:"reason"`
}

type PromptStatus string

const (
	PromptStatusPending   PromptStatus = "pending"
	PromptStatusDelivered PromptStatus = "delivered"
	PromptStatusSettled   PromptStatus = "settled"
)

type Prompt struct {
	RequestID, TurnID, Text, Dispatch, Delivery, RunnerID string
	Status                                                PromptStatus
	Consumed                                              bool
	Reason                                                string
	At                                                    time.Time
}

func Settle(st State, match func(Prompt) bool, consumed bool, reason string) ([]Row, error) {
	var rows []Row
	for _, p := range st.PendingPrompts() {
		if !match(p) {
			continue
		}
		r, err := NewRow(KindPromptSettled, "", "", PromptSettled{RequestID: p.RequestID, Consumed: consumed, Reason: reason})
		if err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, nil
}

func SettleCarried(st State, runnerID string, consumed bool, reason string) ([]Row, error) {
	return Settle(st, func(p Prompt) bool { return p.RunnerID == runnerID }, consumed, reason)
}

func SettleUntaken(st State, match func(Prompt) bool, command func(text string) bool) ([]Row, error) {
	isCommand := func(p Prompt) bool { return command != nil && command(cmp.Or(p.Dispatch, p.Text)) }
	ran, err := Settle(st, func(p Prompt) bool { return match(p) && isCommand(p) }, true, ReasonProviderBuiltin)
	if err != nil {
		return nil, err
	}
	rest, err := Settle(st, func(p Prompt) bool { return match(p) && !isCommand(p) }, false, ReasonNotAcknowledged)
	return append(ran, rest...), err
}

func SettleOvertaken(st State, runnerID string) ([]Row, error) {
	return Settle(st, func(p Prompt) bool {
		return p.RunnerID == runnerID && p.Delivery == DeliveryType
	}, false, ReasonNotAcknowledged)
}

func SettleOrphaned(st State, live func(runnerID string) bool) ([]Row, error) {
	return Settle(st, func(p Prompt) bool { return p.RunnerID != "" && !live(p.RunnerID) }, false, ReasonRunnerExited)
}
