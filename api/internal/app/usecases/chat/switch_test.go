package chat_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// TestRegression_SwitchInsideTheUnacknowledgedPromptWindowIsRefusedAtOnce is the
// 409 reproduced on 2026-09-23: a switch sent between SubmitPrompt's 200 and the
// provider's user_prompt hook, while Working still read false. Nothing settles a
// pending prompt on a clock, so the switch refuses at once and destroys nothing;
// once the provider has taken the prompt and answered it, the switch goes through.
func TestRegression_SwitchInsideTheUnacknowledgedPromptWindowIsRefusedAtOnce(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "reply with only: SEED", uuid.NewString(), "", nil)
	require.NoError(t, err)
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: the prompt is recorded and unacknowledged")
	require.False(t, f.chat(t, chatID).Working,
		"precondition: Working is set by the hook that has not arrived")
	spawns, terminated := f.term.callCount(), len(f.term.terminatedIDs())

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.ErrorIs(t, err, agentusecase.ErrPromptBusy)
	assert.Equal(t, spawns, f.term.callCount(), "a refused switch starts nothing")
	assert.Len(t, f.term.terminatedIDs(), terminated, "and kills nothing: the CLI carrying the prompt survives")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	prompt(t, f, live.ID, "claude", "reply with only: SEED")
	turn(t, f, live.ID, "claude", "SEED")
	require.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"the user_prompt hook acknowledged the prompt")

	switched, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	newLive, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, switched, newLive.ID)
	assert.Equal(t, "codex", newLive.ProviderID)
	incoming := f.term.calls[f.term.callCount()-1]
	assert.Contains(t, strings.Join(incoming.argv, "\x00"), "SEED",
		"the handoff carries the turn answered before the switch")
}
