package domain

import "context"

// AgentPromptSubmission identifies the replacement interactive TUI whose spawn
// made a React prompt submission successful. Completion of the model turn is
// observed later through hooks; it is intentionally not represented here.
type AgentPromptSubmission struct {
	RunnerID          string
	TerminalSessionID string
}

type promptRetryOfKey struct{}

// WithPromptRetryOf marks a send as the retry of the refused prompt requestID. It rides
// the request rather than every submit signature: only the ledger write reads it.
func WithPromptRetryOf(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, promptRetryOfKey{}, requestID)
}

// PromptRetryOf is the refused prompt this send retries, "" for a first send.
func PromptRetryOf(ctx context.Context) string {
	id, _ := ctx.Value(promptRetryOfKey{}).(string)
	return id
}
