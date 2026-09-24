package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubRunnerStoreForProviderResolution answers the two history reads
// lastActiveProviderID makes and nothing else. The embedded nil interface panics
// on any other method, so a test that starts depending on a third read fails
// loudly instead of silently zero-valuing it.
type stubRunnerStoreForProviderResolution struct {
	agentrunner.EventStore
	placementsErr error
}

func (s stubRunnerStoreForProviderResolution) LastConversation(
	context.Context, string,
) (engineagents.ChatConversation, error) {
	return engineagents.ChatConversation{}, agentrunner.ErrNotFound
}

func (s stubRunnerStoreForProviderResolution) PlacementsForChat(
	context.Context, string,
) ([]engineagents.ChatPlacement, error) {
	return nil, s.placementsErr
}

// stubActivityForProviderResolution answers an empty interruption ledger, the
// shape of a chat that was never switched.
type stubActivityForProviderResolution struct {
	agentactivity.EventStore
}

func (stubActivityForProviderResolution) Interruptions(
	context.Context, string,
) ([]domain.ActivityInterruption, error) {
	return nil, nil
}

// A placement read that BLEW UP is not the same fact as a chat with no placement
// history, and must never be flattened into one. Swallowing it would report
// "this chat no longer records which provider it ran" for a chat whose record is
// sitting right there behind a broken read — and the client's correct response to
// that answer is to stop asking, which would make a transient failure look like
// permanent data loss.
func TestLastActiveProviderID_APlacementReadFailurePropagates(t *testing.T) {
	boom := errors.New("projection down")
	rs := &Runners{
		runnerStore: stubRunnerStoreForProviderResolution{placementsErr: boom},
		activity:    stubActivityForProviderResolution{},
	}

	_, err := rs.lastActiveProviderID(context.Background(), "c1")

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrChatProviderUnknown,
		"a broken read must never be reported as a chat that has no provider")
}
