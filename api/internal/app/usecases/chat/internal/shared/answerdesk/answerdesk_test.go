package answerdesk_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// proceeded is what a hooks relay's expiry hands its CLI: nothing, so it asks itself.
const proceeded = domain.ChoiceResolutionProceeded

func fixture(wait time.Duration) (*answerdesk.Desk, *answerdesk.Slot) {
	return fixtureRetaining(wait, answerdesk.DefaultRetention)
}

// recordingLedger remembers each outcome the desk writes back.
type recordingLedger struct {
	mu       sync.Mutex
	resolved []resolution
	err      error
}

type resolution struct{ chatID, choiceID, verdict string }

func (l *recordingLedger) ResolveAsk(_ context.Context, chatID, askID, verdict string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resolved = append(l.resolved, resolution{chatID, askID, verdict})
	return l.err
}

func (l *recordingLedger) all() []resolution {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]resolution(nil), l.resolved...)
}

func withLedger(wait time.Duration, opts ...answerdesk.Option) (*answerdesk.Desk, *answerdesk.Slot, *recordingLedger) {
	ledger := &recordingLedger{}
	d := answerdesk.New(answerdesk.DefaultRetention, ledger, opts...)
	slot := d.Hold(answerdesk.Prompt{
		ChoiceID: "choice-1",
		ChatID:   "chat-1",
		RunnerID: "runner-1",
		Event:    engineagents.HookPermission,
		Keys:     engineagents.AnswerCapability{Wait: wait, Keys: []string{"allow"}},
	})
	return d, slot, ledger
}

// recorded is an answer's record that went through.
func recorded(context.Context) (bool, error) { return true, nil }

// decide is a person's answer to slot, recorded and handed to its relay.
func decide(d *answerdesk.Desk, slot *answerdesk.Slot, stdout string) error {
	_, err := d.Answer(context.Background(), slot, []byte(stdout), recorded)
	return err
}

func fixtureRetaining(wait, retention time.Duration) (*answerdesk.Desk, *answerdesk.Slot) {
	d := answerdesk.New(retention, nil)
	slot := d.Hold(answerdesk.Prompt{
		ChoiceID: "choice-1",
		ChatID:   "chat-1",
		RunnerID: "runner-1",
		Event:    engineagents.HookPermission,
		Keys:     engineagents.AnswerCapability{Wait: wait, Keys: []string{"allow"}},
	})
	return d, slot
}

func TestWait_ClampsAnAbsentOrAbsurdBudget(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 120*time.Second, answerdesk.Wait(0))
	assert.Equal(t, 120*time.Second, answerdesk.Wait(-time.Second))
	assert.Equal(t, 15*time.Minute, answerdesk.Wait(time.Hour))
	assert.Equal(t, 30*time.Second, answerdesk.Wait(30*time.Second))
}

func TestPending_ReportsTheChoiceAndTheClampedBudget(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Hour)

	pending, held := d.Pending("choice-1")

	require.True(t, held)
	assert.Equal(t, "choice-1", pending.ChoiceID)
	assert.Equal(t, 15*time.Minute, pending.Wait, "the relay is told the clamped budget, not the declared one")
}

func TestHold_ReopeningAPromptReleasesTheRelayAlreadyHoldingIt(t *testing.T) {
	t.Parallel()

	d, stale := fixture(time.Minute)

	fresh := d.Hold(answerdesk.Prompt{
		ChoiceID: "choice-1", ChatID: "chat-1", RunnerID: "runner-1",
	})

	<-stale.Settled()
	held, ok := d.ByChoiceID("choice-1")
	require.True(t, ok)
	assert.Same(t, fresh, held)
	require.ErrorIs(t, decide(d, stale, "allow"), answerdesk.ErrNotWaiting)
	_, stillOpen := d.ByChoiceID("choice-1")
	assert.True(t, stillOpen, "a verdict on the displaced relay answers nothing")
}

func TestReleaseRunner_FreesOnlyThatRunnersRelays(t *testing.T) {
	t.Parallel()

	d, mine := fixture(time.Minute)
	theirs := d.Hold(answerdesk.Prompt{
		ChoiceID: "choice-2", ChatID: "chat-2", RunnerID: "runner-2",
	})

	released := d.ReleaseRunner("runner-1")

	require.Len(t, released, 1)
	assert.Same(t, mine, released[0])
	assert.Equal(t, "chat-1", released[0].ChatID)
	assert.Equal(t, "choice-1", released[0].ChoiceID)
	<-mine.Settled()
	_, stillHeld := d.ByChoiceID("choice-2")
	assert.True(t, stillHeld, "another runner's relay must not be collateral")
	select {
	case <-theirs.Settled():
		t.Fatal("another runner's relay was released")
	default:
	}
}

// A relay whose verdict already landed was answered, not abandoned: releasing its
// dead runner must not reopen a question the user already decided.
func TestReleaseRunner_DoesNotReportAnAlreadyDecidedRelay(t *testing.T) {
	t.Parallel()

	d, slot := fixture(time.Minute)
	require.NoError(t, decide(d, slot, "allow"))

	released := d.ReleaseRunner("runner-1")

	assert.Empty(t, released, "a decided relay is not an abandoned one")
	_, waiting := d.Pending("choice-1")
	assert.False(t, waiting, "but its slot is still freed")
}

func TestAbandon_ReportsTheQuestionToCloseWhenNobodyDecided(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Minute)

	d.Abandon(context.Background(), "choice-1")

	_, waiting := d.Pending("choice-1")
	assert.False(t, waiting)
}

func TestAbandon_ClosesNothingWhenTheVerdictWasAlreadyIn(t *testing.T) {
	t.Parallel()

	d, slot, ledger := withLedger(time.Minute)
	require.NoError(t, decide(d, slot, "allow"))

	d.Abandon(context.Background(), "choice-1")

	assert.Empty(t, ledger.all(), "the question was decided, not proceeded past")
}

func TestAbandon_IsSilentForAnAskNobodyIsHolding(t *testing.T) {
	t.Parallel()

	ledger := &recordingLedger{}
	d := answerdesk.New(answerdesk.DefaultRetention, ledger)

	d.Abandon(context.Background(), "never-held")

	assert.Empty(t, ledger.all())
}

func TestAnswerableIDs_KeepsOnlyTheChoicesThisChatStillHolds(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Minute)
	d.Hold(answerdesk.Prompt{ChoiceID: "choice-2", ChatID: "chat-2", RunnerID: "runner-2"})

	ids := d.AnswerableIDs("chat-1", choices("choice-1", "choice-2", "choice-3"))

	assert.Equal(t, []string{"choice-1"}, ids,
		"another chat's held prompt and a prompt nobody holds are both unanswerable")
}

func TestAnswerableIDs_IsEmptyRatherThanNilForNoChoices(t *testing.T) {
	t.Parallel()

	d := answerdesk.New(answerdesk.DefaultRetention, nil)

	assert.Empty(t, d.AnswerableIDs("chat-1", nil))
}

func TestLedger_AbandonClosesTheQuestionAsProceeded(t *testing.T) {
	t.Parallel()

	d, _, ledger := withLedger(time.Minute)

	d.Abandon(context.Background(), "choice-1")

	require.Equal(t, []resolution{{"chat-1", "choice-1", domain.ChoiceResolutionProceeded}}, ledger.all(),
		"an undecided prompt is being resolved by the provider's own UI")
}

// A dead runner's exit closes its questions; releasing its relays writes nothing.
func TestLedger_ADeadRunnersReleaseWritesNothing(t *testing.T) {
	t.Parallel()

	d, slot, ledger := withLedger(time.Minute)

	d.ReleaseRunner("runner-1")

	<-slot.Settled()
	assert.Empty(t, ledger.all())
}

// A ledger that refuses must not strand the relay it already released.
func TestLedger_AFailedWriteStillReleasesTheRelay(t *testing.T) {
	t.Parallel()

	ledger := &recordingLedger{err: errors.New("store is down")}
	d := answerdesk.New(answerdesk.DefaultRetention, ledger)
	slot := d.Hold(answerdesk.Prompt{ChoiceID: "choice-1", ChatID: "chat-1", RunnerID: "runner-1"})

	d.Abandon(context.Background(), "choice-1")

	<-slot.Settled()
	_, waiting := d.Pending("choice-1")
	assert.False(t, waiting)
}

// A desk with no ledger records nothing and refuses nothing.
func TestLedger_NilLedgerIsSilent(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Minute)

	d.Abandon(context.Background(), "choice-1")
	d.ReleaseRunner("runner-1")
}

func TestWithChange_AnnouncesEachMoveOfTheAnswerableSetAfterTheLockIsReleased(t *testing.T) {
	t.Parallel()

	var d *answerdesk.Desk
	var announced []string
	d = answerdesk.New(answerdesk.DefaultRetention, nil, answerdesk.WithChange(func(chatID string) {
		// Reading the desk back would deadlock if it were announced under its lock.
		_ = d.AnswerableIDs(chatID, nil)
		announced = append(announced, chatID)
	}))

	slot := d.Hold(answerdesk.Prompt{ChoiceID: "choice-1", ChatID: "chat-1"})
	require.NoError(t, decide(d, slot, "allow"))
	other := d.Hold(answerdesk.Prompt{ChoiceID: "choice-2", ChatID: "chat-2"})
	d.Discard(other)

	assert.Equal(t, []string{"chat-1", "chat-1", "chat-2", "chat-2"}, announced)
}

func TestWithChange_AReleasedRelayAndADeadRunnerAreAnnounced(t *testing.T) {
	t.Parallel()

	var announced []string
	d := answerdesk.New(answerdesk.DefaultRetention, nil, answerdesk.WithChange(func(chatID string) {
		announced = append(announced, chatID)
	}))
	d.Hold(answerdesk.Prompt{
		ChoiceID: "choice-1", ChatID: "chat-1", RunnerID: "runner-1",
		Keys: engineagents.AnswerCapability{Wait: time.Millisecond},
	})
	d.Hold(answerdesk.Prompt{ChoiceID: "choice-2", ChatID: "chat-2", RunnerID: "runner-2"})
	announced = nil

	_, err := d.Await(context.Background(), "choice-1", proceeded)
	require.NoError(t, err)
	d.ReleaseRunner("runner-2")

	assert.Equal(t, []string{"chat-1", "chat-2"}, announced)
}
