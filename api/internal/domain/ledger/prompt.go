package ledger

import "time"

// Who a user_message speaks for.
const (
	OriginCrowbar  = "crowbar"  // a prompt Crowbar delivered for its user
	OriginProvider = "provider" // text typed into the CLI itself
	OriginHarness  = "harness"  // the provider's own harness speaking as the user
)

// How Crowbar hands a prompt to the CLI.
const (
	DeliveryRestart = "restart" // a replacement CLI launched with the prompt
	DeliverySteer   = "steer"   // parked on the running turn's end hook
	DeliveryAPI     = "api"     // pushed over the live api connection
)

// Why a prompt ended without a turn.
const (
	ReasonProviderBuiltin = "provider_builtin" // a provider builtin consumed it (consumed)
	ReasonNotAcknowledged = "not_acknowledged"
	ReasonRunnerExited    = "runner_exited"
	ReasonDaemonRestart   = "daemon_restart"
	ReasonDeliveryFailed  = "delivery_failed"
	ReasonSessionLost     = "session_lost"
)

// UserMessage is the body of a user_message row.
type UserMessage struct {
	RequestID string `json:"requestId,omitempty"`
	Text      string `json:"text"`
	Origin    string `json:"origin"`             // OriginCrowbar | OriginProvider | OriginHarness
	Delivery  string `json:"delivery,omitempty"` // crowbar origin only
	// RunnerID is the CLI carrying a crowbar prompt: only its exit strands it.
	RunnerID string `json:"runnerId,omitempty"`
}

// TurnStarted is the body of a turn_started row: the provider took the turn,
// on the session the chat was bound to with it.
type TurnStarted struct {
	RequestID      string `json:"requestId,omitempty"` // crowbar origin only
	ProviderTurnID string `json:"providerTurnId,omitempty"`
	RunnerID       string `json:"runnerId,omitempty"`
	Provider       string `json:"provider,omitempty"`
	SessionID      string `json:"sessionId,omitempty"`
	// Persists is when the provider's CLI writes the session: PersistsTurnStart or PersistsTurnEnd.
	Persists string `json:"persists,omitempty"`
}

// PromptSettled is the body of a prompt_settled row: a crowbar prompt ended
// without a turn.
type PromptSettled struct {
	RequestID string `json:"requestId"`
	Consumed  bool   `json:"consumed"`
	Reason    string `json:"reason"`
}

// PromptStatus is where a crowbar prompt is in its life.
type PromptStatus string

const (
	PromptStatusPending   PromptStatus = "pending"   // recorded, not yet acknowledged
	PromptStatusDelivered PromptStatus = "delivered" // a turn started for it
	PromptStatusSettled   PromptStatus = "settled"   // ended without a turn
)

// Prompt is one crowbar prompt as the ledger has folded it.
type Prompt struct {
	RequestID, TurnID, Text, Delivery, RunnerID string
	Status                                      PromptStatus
	Consumed                                    bool
	Reason                                      string
	At                                          time.Time
}

// Settle is a prompt_settled row for every pending prompt match selects.
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

// SettleCarried settles the pending prompts runnerID carries: a runner that has
// left the chat can acknowledge none of them.
func SettleCarried(st State, runnerID string, consumed bool, reason string) ([]Row, error) {
	return Settle(st, func(p Prompt) bool { return p.RunnerID == runnerID }, consumed, reason)
}

// SettleOrphaned settles, as runner_exited, every pending prompt whose carrier
// live does not report. A prompt with no carrier is never orphaned.
func SettleOrphaned(st State, live func(runnerID string) bool) ([]Row, error) {
	return Settle(st, func(p Prompt) bool { return p.RunnerID != "" && !live(p.RunnerID) }, false, ReasonRunnerExited)
}
