package runner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubChatsForOrigin counts chat creation — the one thing a session Crowbar
// originated itself must never cause.
type stubChatsForOrigin struct {
	agentchat.EventStore
	chat    domain.Chat
	created int
}

func (s *stubChatsForOrigin) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

func (s *stubChatsForOrigin) Create(
	_ context.Context, in agentchat.CreateInput,
) (domain.Chat, error) {
	s.created++
	return domain.Chat{ID: in.ID, WorkspaceID: in.WorkspaceID}, nil
}

// stubRunnerStoreForOrigin records which placement call the ingest chose. A
// conversation nobody knows resolves to ErrNotFound, exactly as it does for a
// real /clear.
type stubRunnerStoreForOrigin struct {
	agentrunner.EventStore
	runner      engineagents.Runner
	boundTo     string
	movedToChat string
}

func (s *stubRunnerStoreForOrigin) ChatForSession(
	context.Context, string, string,
) (string, error) {
	return "", agentrunner.ErrNotFound
}

func (s *stubRunnerStoreForOrigin) LiveRunnerForChat(
	context.Context, string,
) (engineagents.Runner, error) {
	return s.runner, nil
}

func (s *stubRunnerStoreForOrigin) LiveRunnersForSession(
	context.Context, string, string,
) ([]engineagents.Runner, error) {
	return nil, nil
}

func (s *stubRunnerStoreForOrigin) BindSession(
	_ context.Context, _, sessionID string, _ bool, _ time.Time, _, _ string,
) (engineagents.Runner, error) {
	s.boundTo = sessionID
	return s.runner, nil
}

func (s *stubRunnerStoreForOrigin) Move(
	_ context.Context, _, toChatID, _ string, _ bool, _ time.Time, _, _ string,
) (engineagents.Runner, error) {
	s.movedToChat = toChatID
	return s.runner, nil
}

type stubConversationsForOrigin struct {
	Conversations
}

func (stubConversationsForOrigin) SeedPermissionLevel(context.Context, string) {}

func (stubConversationsForOrigin) ChatTurns(
	context.Context, string,
) ([]domain.LedgerTurn, error) {
	return nil, nil
}

func newOriginFixture(t *testing.T) (*Runners, *stubChatsForOrigin, *stubRunnerStoreForOrigin) {
	t.Helper()
	home := t.TempDir()
	runner := engineagents.Runner{
		ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "codex",
		CurrentChatID: "chat-1", CurrentSession: "the-session-the-provider-forgot",
	}
	chats := &stubChatsForOrigin{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}}
	store := &stubRunnerStoreForOrigin{runner: runner}
	rs := &Runners{
		chats:         chats,
		runnerStore:   store,
		conversations: stubConversationsForOrigin{},
		apiConns:      newAPIConnRegistry(),
		work:          inflight.NewWork(),
		inflightTurns: inflight.NewTurns(),
		prompts:       agentjournal.NewPromptRequests(),
		home:          func() (string, error) { return home, nil },
	}
	return rs, chats, store
}

func originRunner() engineagents.Runner {
	return engineagents.Runner{
		ID: "runner-1", WorkspaceID: "ws-1", ProviderID: "codex",
		CurrentChatID: "chat-1", CurrentSession: "the-session-the-provider-forgot",
	}
}

// TestRegression_ADriverOriginatedSessionBindsToTheChatItIsAlreadyOn is the
// incident: a prompt was refused because the provider had paged the chat's
// conversation out, the driver recovered onto another one, and the new id
// arrived at the hook ingress as a conversation Crowbar had never seen. That is
// indistinguishable from a user-typed /clear by ABSENCE alone, so Crowbar minted
// a fresh chat, titled it with the user's own prompt text, and delivered the
// message there — while the real chat was left wedged. Confirmed live.
//
// A conversation Crowbar's own driver produced belongs to the chat the runner is
// already on. Nothing else about the announcement distinguishes the two.
func TestRegression_ADriverOriginatedSessionBindsToTheChatItIsAlreadyOn(t *testing.T) {
	rs, chats, store := newOriginFixture(t)
	originated := newOriginatedSessions()
	rs.apiConns.set("runner-1", &apiconn{originated: originated})
	originated.Claim()("the-replacement")

	err := rs.HandleSessionStart(context.Background(), originRunner(),
		engineagents.CanonicalEvent{SessionID: "the-replacement"})

	require.NoError(t, err)
	assert.Zero(t, chats.created, "a session Crowbar itself originated must never mint a chat")
	assert.Equal(t, "the-replacement", store.boundTo, "it belongs to the chat the runner is on")
	assert.Empty(t, store.movedToChat)
}

// TestRegression_AnOriginatedSessionAnnouncedBeforeItsCallReturnsStillBinds is
// the race the claim exists for. The provider announces the new conversation
// over the SAME connection the recovery is running on, and can do so before the
// call that created it has returned — so the id is not knowable yet. Claiming
// only after the fact loses that ordering, and the announcement lands while
// Crowbar still believes the runner is on the dead conversation.
func TestRegression_AnOriginatedSessionAnnouncedBeforeItsCallReturnsStillBinds(t *testing.T) {
	rs, chats, store := newOriginFixture(t)
	originated := newOriginatedSessions()
	rs.apiConns.set("runner-1", &apiconn{originated: originated})
	settle := originated.Claim() // the recovery is still in flight: no id yet

	err := rs.HandleSessionStart(context.Background(), originRunner(),
		engineagents.CanonicalEvent{SessionID: "announced-before-the-reply"})

	require.NoError(t, err)
	assert.Zero(t, chats.created, "an announcement arriving mid-recovery is still Crowbar's own doing")
	assert.Equal(t, "announced-before-the-reply", store.boundTo)
	settle("announced-before-the-reply")
}

// TestHandleSessionStart_AGenuineClearStillMintsANewChat keeps the fix narrow:
// with no claim and no record, an announced conversation IS the user typing
// /clear, and that must still open a chat of its own.
func TestHandleSessionStart_AGenuineClearStillMintsANewChat(t *testing.T) {
	rs, chats, store := newOriginFixture(t)
	rs.apiConns.set("runner-1", &apiconn{originated: newOriginatedSessions()})

	err := rs.HandleSessionStart(context.Background(), originRunner(),
		engineagents.CanonicalEvent{SessionID: "the-user-typed-clear"})

	require.NoError(t, err)
	assert.Equal(t, 1, chats.created)
	assert.NotEmpty(t, store.movedToChat)
	assert.NotEqual(t, "chat-1", store.movedToChat)
}
