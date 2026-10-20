package runner

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// refusedS1 is a CLI's exit on refusing to resume session S1.
var refusedS1 = engineterminal.Exit{Code: 1, Screen: "No conversation found with session ID: S1\n"}

// refusingTerm is a PTY seam whose every CLI refuses the session it resumes:
// during its fork when early, else once it is asked to die or refuseAll runs.
type refusingTerm struct {
	fakeTermForAttach
	early bool
}

func (r *refusingTerm) CreateCommand(
	ctx context.Context, chatID, cwd string, argv, env []string, onExit func(engineterminal.Exit),
) (string, error) {
	id, err := r.fakeTermForAttach.CreateCommand(ctx, chatID, cwd, argv, env, onExit)
	if err == nil && r.early {
		r.refuse(id)
	}
	return id, err
}

func (r *refusingTerm) TerminateGraceful(_ context.Context, sessionID string) error {
	if !r.refuse(sessionID) {
		return engineterminal.ErrSessionNotFound
	}
	return nil
}

// refuse ends sessionID's CLI on its refusal, once.
func (r *refusingTerm) refuse(sessionID string) bool {
	r.mu.Lock()
	onExit, ok := r.onExit[sessionID]
	delete(r.onExit, sessionID)
	r.mu.Unlock()
	if ok {
		onExit(refusedS1)
	}
	return ok
}

func (r *refusingTerm) refuseAll() {
	r.mu.Lock()
	ids := slices.Collect(maps.Keys(r.onExit))
	r.mu.Unlock()
	for _, id := range ids {
		r.refuse(id)
	}
}

// reviveConversations is the record of a claude chat with no lineage,
// selection or gap to hand over.
type reviveConversations struct{ stubConversationsForSpawn }

func (reviveConversations) ChatProviderID(context.Context, string) (string, error) {
	return "claude", nil
}

func (reviveConversations) AssembleConversation(context.Context, string, bool, time.Time) (string, error) {
	return "", nil
}

// reviveActivity is a conversation record with nothing in it since the
// session's last turn.
type reviveActivity struct{ stubActivityForSpawn }

func (reviveActivity) LastTurnForSession(context.Context, string, string, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (reviveActivity) TurnsSince(context.Context, string, time.Time) ([]domain.ActivityTurn, error) {
	return nil, nil
}

func (reviveActivity) HasTurnAtOrAfter(context.Context, string, string, time.Time) (bool, error) {
	return false, nil
}

// A send to a dormant chat launches one CLI, carrying the prompt. When that
// CLI refuses the session, its refusal settles the prompt and loses the
// session once, whether it lands before the launch answers or after.
func TestSubmitPromptWithSwitch_ARefusedReviveSettlesThePromptItCarried(t *testing.T) {
	for name, early := range map[string]bool{"before the launch answers": true, "after it answers": false} {
		t.Run(name, func(t *testing.T) {
			h := newSpawnHarness(t, "")
			term := &refusingTerm{early: early}
			h.rs.term = term
			h.rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: h.chatID, WorkspaceID: "ws-1", ProviderID: "claude"}}
			h.rs.conversations, h.rs.activity = reviveConversations{}, reviveActivity{}
			h.rs.ledger = turnedTestLedger(t, "claude", "S1")
			requestID := uuid.NewString()

			_, _ = h.rs.SubmitPromptWithSwitch(t.Context(), h.chatID, "hello", requestID, "", nil)
			term.refuseAll()

			assert.Equal(t, 1, term.callCount(), "the send launches one CLI")
			argv := term.lastCall().argv
			assert.Equal(t, "S1", argv[slices.Index(argv, "--resume")+1], "%v", argv)
			assert.Contains(t, argv, "hello", "the CLI it launches carries the prompt")
			p := promptReceipt(t, h.rs.ledger, requestID)
			assert.Equal(t, ledger.PromptStatusSettled, p.Status)
			assert.Equal(t, ledger.ReasonSessionLost, p.Reason)
			assert.False(t, p.Consumed)
			st := chatState(t, h.rs.ledger)
			exit, _ := st.LastExit()
			assert.Equal(t, domain.AgentExitResumeFailed, exit.Reason)
			c, _ := st.Conversation("claude")
			assert.Equal(t, ledger.ConversationLost, c.State)
			rows, err := h.rs.ledger.Read(t.Context(), h.chatID, 0, 0)
			require.NoError(t, err)
			assert.Equal(t, 1, countKind(rows, ledger.KindSessionLost))
		})
	}
}
