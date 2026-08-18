package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// trustDialog is claude's workspace-trust screen, from the capture recorded in
// tests/integration/agent/barriers_test.go.
const trustDialog = "❯ 1. Yes, I trust this folder\n  2. No, exit\n  Enter to confirm · Esc to cancel"

// TestUsecase_MatchTerminalPrompt_ResolvesTheShippedDescriptor drives the seam the
// detector reaches provider vocabulary through — home resolution, descriptor
// lookup, needle match — against the real embedded catalogue.
func TestUsecase_MatchTerminalPrompt_ResolvesTheShippedDescriptor(t *testing.T) {
	f := newFixture(t)

	prompt, ok := f.usecase.MatchTerminalPrompt(f.ctx, "claude", trustDialog)

	require.True(t, ok)
	assert.Equal(t, domain.AgentTerminalWaitTrust, prompt.Kind)
}

// TestUsecase_MatchTerminalPrompt_UnknownProviderIsSilent: an unresolvable
// descriptor declares no needles, and a provider that declares none never reports
// waiting. Silence rather than an error, because a caller sweeping every live
// runner must not be able to fail on one it cannot identify.
func TestUsecase_MatchTerminalPrompt_UnknownProviderIsSilent(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalPrompt(f.ctx, "telepathy", trustDialog)

	assert.False(t, ok)
}

func TestUsecase_MatchTerminalPrompt_OrdinaryScreenIsNotABlock(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalPrompt(f.ctx, "claude", "> Ready.\n  shift+tab to cycle")

	assert.False(t, ok)
}
