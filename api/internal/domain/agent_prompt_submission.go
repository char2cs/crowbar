package domain

import "context"

type AgentPromptSubmission struct {
	RunnerID          string
	TerminalSessionID string
}

type promptRetryOfKey struct{}

func WithPromptRetryOf(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, promptRetryOfKey{}, requestID)
}

func PromptRetryOf(ctx context.Context) string {
	id, _ := ctx.Value(promptRetryOfKey{}).(string)
	return id
}
