package runner

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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

// stagedConversations is reviveConversations holding the chat's pick and
// keeping the picks a send commits.
type stagedConversations struct {
	reviveConversations
	committed *[]string
	held      engineagents.Selection
}

func (c stagedConversations) ChatSelection(context.Context, string, bool) (engineagents.Selection, error) {
	return c.held, nil
}

func (c stagedConversations) SetChatSelection(_ context.Context, _, model, effort string) error {
	*c.committed = append(*c.committed, model+"/"+effort)
	return nil
}

// A pick staged with a switch rides the switch's launch, and is the chat's,
// dividers and all, once that launch runs: a failed one leaves the chat as it was.
func TestSubmitPromptWithSwitch_AStagedPickIsTheChatsOnceItsLaunchRuns(t *testing.T) {
	for name, runs := range map[string]bool{"the launch runs": true, "the launch fails": false} {
		t.Run(name, func(t *testing.T) {
			h := newSpawnHarness(t, "")
			installDescriptor(t, h.home, selectionCarrierDescriptor)
			var committed []string
			h.rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: h.chatID, WorkspaceID: "ws-1", ProviderID: "claude"}}
			h.rs.conversations = stagedConversations{reviveConversations: reviveConversations{provider: "claude"}, committed: &committed}
			if !runs {
				h.rs.term = unforkable{}
			}

			_, err := h.rs.SubmitPromptWithSwitch(t.Context(), h.chatID, "hello", uuid.NewString(),
				"carrier-test", &domain.ChatSelection{Model: "deep"})

			if !runs {
				require.Error(t, err)
				assert.Empty(t, committed, "the chat keeps its own pick")
				assert.Empty(t, dividers(t, h.rs.ledger, h.chatID), "and no divider records a switch that never ran")
				return
			}
			require.NoError(t, err)
			assert.Contains(t, h.term.lastCall().argv, "deep", "the launch carries the staged pick")
			assert.Equal(t, []string{"deep/"}, committed)
			assert.Equal(t, []string{
				ledger.MarkerProviderSwitched + ":carrier-test", ledger.MarkerModelChanged + ":deep",
			}, dividers(t, h.rs.ledger, h.chatID))
		})
	}
}

// A revive whose pick is the chat's own writes nothing: the composer re-posts
// its pick with every send.
func TestSubmitPromptWithSwitch_ARevivesUnchangedPickCommitsNothing(t *testing.T) {
	h := newSpawnHarness(t, "")
	installDescriptor(t, h.home, selectionCarrierDescriptor)
	var committed []string
	h.rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: h.chatID, WorkspaceID: "ws-1", ProviderID: "carrier-test"}}
	h.rs.conversations = stagedConversations{
		reviveConversations: reviveConversations{provider: "carrier-test"}, committed: &committed,
		held: engineagents.Selection{Model: "deep"},
	}

	_, err := h.rs.SubmitPromptWithSwitch(t.Context(), h.chatID, "hello", uuid.NewString(), "",
		&domain.ChatSelection{Model: "deep"})

	require.NoError(t, err)
	assert.Contains(t, h.term.lastCall().argv, "deep", "the revive runs the chat's pick")
	assert.Empty(t, committed, "an unchanged pick is not written again")
	assert.Empty(t, dividers(t, h.rs.ledger, h.chatID))
}

// A selection's dividers land after the hook holding its chat: the change
// waits for the chat's lock, so they never cut into that hook's record.
func TestSetChatSelection_ItsDividersLandAfterTheHookHoldingTheChat(t *testing.T) {
	locks, asked := ingress.NewLocks(), make(chan string, 1)
	var committed []string
	rs := &Runners{
		chatLock: askedLock{Locks: locks, asked: asked}, parks: newParks(), ledger: newTestLedger(t),
		conversations: stagedConversations{committed: &committed},
	}
	unlock, err := locks.Lock(t.Context(), "chat-1") // a hook mid-record
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- rs.SetChatSelection(context.Background(), "chat-1", "deep", "") }()
	select {
	case <-asked:
	case err := <-done:
		done <- err
	}
	said, err := ledger.NewRow(ledger.KindAssistantText, "T-1", "", ledger.AssistantText{MessageID: "m-1", Text: "said"})
	require.NoError(t, err)
	_, _, err = rs.ledger.Append(t.Context(), "chat-1", said)
	require.NoError(t, err)
	unlock()
	require.NoError(t, <-done)

	rows, err := rs.ledger.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, ledger.KindAssistantText, rows[0].Kind)
	assert.Equal(t, ledger.KindMarker, rows[1].Kind, "the divider follows the hook's row")
}
