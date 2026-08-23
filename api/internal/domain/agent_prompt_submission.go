package domain

// AgentPromptSubmission identifies the replacement interactive TUI whose spawn
// made a React prompt submission successful. Completion of the model turn is
// observed later through hooks; it is intentionally not represented here.
type AgentPromptSubmission struct {
	RunnerID          string
	TerminalSessionID string
}
