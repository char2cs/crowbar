package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// stubChatsForSelection answers only GetChat — the one call
// validateSelectionForProvider makes on its way to resolving the chat's
// worktree, same panic-on-anything-else pattern as this package's other
// narrow stubs.
type stubChatsForSelection struct {
	agentchat.EventStore
	chat domain.Chat
}

func (s stubChatsForSelection) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

func runnersForSelectionTest(t *testing.T, agent engineagents.Agent) *Runners {
	t.Helper()
	return &Runners{
		chats:  stubChatsForSelection{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}},
		ws:     stubWorkspaceForInterrupt{crowbarHome: t.TempDir()},
		agents: stubAgentsForInterrupt{agent: agent},
	}
}

func TestValidateSelectionForProvider_AcceptsAModelTheResolvedCatalogueDeclares(t *testing.T) {
	rs := runnersForSelectionTest(t, fakeSelectionAgent{models: []string{"opus", "sonnet"}, efforts: []string{"low", "high"}})

	err := rs.validateSelectionForProvider(context.Background(), "chat-1", "some-provider", "opus", "high")

	require.NoError(t, err)
}

func TestValidateSelectionForProvider_RejectsAModelAResolvedCatalogueOmits(t *testing.T) {
	rs := runnersForSelectionTest(t, fakeSelectionAgent{models: []string{"opus", "sonnet"}})

	err := rs.validateSelectionForProvider(context.Background(), "chat-1", "some-provider", "not-a-real-model", "")

	require.Error(t, err)
}

// The required property: an empty/unresolved discovery must never reject a
// non-empty model.
func TestValidateSelectionForProvider_AnUnresolvedDiscoveryAcceptsAnyModel(t *testing.T) {
	rs := runnersForSelectionTest(t, fakeSelectionAgent{discovered: true})

	err := rs.validateSelectionForProvider(context.Background(), "chat-1", "some-provider", "gpt-6-astra", "ultra")

	require.NoError(t, err)
}

func TestValidateSelectionForProvider_AStaticEmptyCatalogueStillRejects(t *testing.T) {
	rs := runnersForSelectionTest(t, fakeSelectionAgent{discovered: false})

	err := rs.validateSelectionForProvider(context.Background(), "chat-1", "some-provider", "anything", "")

	require.Error(t, err, "no catalogue at all (not discovery-backed) must keep rejecting, same as before")
}
