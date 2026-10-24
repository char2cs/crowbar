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

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
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

// reviveConversations is the record of a chat on provider with no lineage,
// selection or gap to hand over.
type reviveConversations struct {
	stubConversationsForSpawn
	provider string
}

func (c reviveConversations) ChatProviderID(context.Context, string) (string, error) {
	return c.provider, nil
}

func (reviveConversations) AssembleConversation(context.Context, string, bool, time.Time) (string, error) {
	return "", nil
}

func (reviveConversations) ChatTurns(context.Context, string) ([]domain.LedgerTurn, error) {
	return nil, nil
}

// dividers are the dividers chatID's ledger holds, as kind:detail.
func dividers(t *testing.T, l *ledgerstore.Store, chatID string) []string {
	t.Helper()
	rows, err := l.Read(t.Context(), chatID, 0, 0)
	require.NoError(t, err)
	var out []string
	for _, r := range rows {
		if r.Kind == ledger.KindMarker {
			body, err := ledger.DecodeBody[ledger.Marker](r)
			require.NoError(t, err)
			out = append(out, body.Kind+":"+body.Detail)
		}
	}
	return out
}

// A send launches one CLI carrying the prompt: the dormant chat's provider or
// the one staged with it. That CLI's refusal of the session, before the launch
// answers or after, settles the prompt and loses the session once.
func TestSubmitPromptWithSwitch_ARefusedResumeSettlesThePromptItsOneLaunchCarried(t *testing.T) {
	scenarios := map[string]struct {
		current, staged string
		live            bool
	}{
		"a dormant chat":                      {current: "claude"},
		"a dormant chat with a staged switch": {current: "codex", staged: "claude"},
		"a live chat with a staged switch":    {current: "codex", staged: "claude", live: true},
	}
	for name, sc := range scenarios {
		for order, early := range map[string]bool{"before the launch answers": true, "after it answers": false} {
			t.Run(name+", "+order, func(t *testing.T) {
				h := newSpawnHarness(t, "")
				term := &refusingTerm{}
				h.rs.term = term
				h.rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: h.chatID, WorkspaceID: "ws-1", ProviderID: sc.current}}
				h.rs.conversations = reviveConversations{provider: sc.current}
				h.rs.ledger = turnedTestLedger(t, "claude", "S1")
				if sc.live {
					_, err := h.rs.spawnRunner(t.Context(), h.chatID, "ws-1", sc.current, spawnInput{}, false)
					require.NoError(t, err)
				}
				term.early = early
				launched := term.callCount()
				requestID := uuid.NewString()

				submission, err := h.rs.SubmitPromptWithSwitch(t.Context(), h.chatID, "hello", requestID, sc.staged, nil)
				term.refuseAll()

				if early {
					require.ErrorIs(t, err, ErrPromptOutcomeUnknown)
				} else {
					require.NoError(t, err)
					assert.NotEmpty(t, submission.RunnerID)
				}
				assert.Equal(t, 1, term.callCount()-launched, "the send launches one CLI")
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
				var divider []string
				if sc.staged != "" {
					divider = []string{ledger.MarkerProviderSwitched + ":claude"}
				}
				assert.Equal(t, divider, dividers(t, h.rs.ledger, h.chatID), "the switch is recorded where the chat's provider moves")
			})
		}
	}
}
