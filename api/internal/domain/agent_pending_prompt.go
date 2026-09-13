package domain

// PendingPrompt is the most recent prompt submission a chat's journal has not
// confirmed the provider accepted — recovered so a client whose own copy of
// the text was lost (an idle tab, a crash, cleared local storage) can show it
// back to the user instead of losing it outright.
type PendingPrompt struct {
	Text  string
	State string
	// RequestID is the original client request id this submission was journalled
	// under (agentjournal.PromptRequest.RequestID) — not freshly minted, so a
	// recovered row stays inside this subsystem's own at-most-once dedup and
	// matches the broadcasts that settle or abandon it.
	RequestID string
}
