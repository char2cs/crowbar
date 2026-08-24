package answerdesk_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func fixture(wait time.Duration) (*answerdesk.Desk, *answerdesk.Slot) {
	return fixtureRetaining(wait, answerdesk.DefaultRetention)
}

// recordingLedger is the conversation record the desk writes each outcome back
// to. Nothing else in these tests needs a store, so it is the whole of the
// production shape: one call, remembered.
type recordingLedger struct {
	mu       sync.Mutex
	resolved []resolution
	err      error
}

type resolution struct{ chatID, choiceID, verdict string }

func (l *recordingLedger) ResolveChoice(
	_ context.Context,
	chatID, choiceID, verdict string,
	_ time.Time,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resolved = append(l.resolved, resolution{chatID, choiceID, verdict})
	return l.err
}

func (l *recordingLedger) all() []resolution {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]resolution(nil), l.resolved...)
}

func withLedger(wait time.Duration) (*answerdesk.Desk, *answerdesk.Slot, *recordingLedger) {
	ledger := &recordingLedger{}
	d := answerdesk.New(answerdesk.DefaultRetention, ledger)
	slot := d.Hold("delivery-1", answerdesk.Prompt{
		ChoiceID: "choice-1",
		ChatID:   "chat-1",
		RunnerID: "runner-1",
		Event:    engineagents.HookPermission,
		Keys:     engineagents.AnswerCapability{Wait: wait, Keys: []string{"allow"}},
	})
	return d, slot, ledger
}

func fixtureRetaining(wait, retention time.Duration) (*answerdesk.Desk, *answerdesk.Slot) {
	d := answerdesk.New(retention, nil)
	slot := d.Hold("delivery-1", answerdesk.Prompt{
		ChoiceID: "choice-1",
		ChatID:   "chat-1",
		RunnerID: "runner-1",
		Event:    engineagents.HookPermission,
		Keys:     engineagents.AnswerCapability{Wait: wait, Keys: []string{"allow"}},
	})
	return d, slot
}

func TestAwait_TheBudgetExpiringPrintsNothingAndFreesTheSlot(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Millisecond)

	answer, err := d.Await(context.Background(), "delivery-1")

	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
	_, waiting := d.Pending("delivery-1")
	assert.False(t, waiting, "an expired wait must not leave a slot an answer could reach")
}

func TestAwait_ACancelledRelayIsFreedAndReportsTheCancellation(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	answer, err := d.Await(ctx, "delivery-1")

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, answer.Stdout)
	_, waiting := d.Pending("delivery-1")
	assert.False(t, waiting, "a cancelled relay must not leave its slot on the desk")
}

func TestAwait_ADeliveryNobodyIsHoldingReturnsNothing(t *testing.T) {
	t.Parallel()

	d := answerdesk.New(answerdesk.DefaultRetention, nil)

	answer, err := d.Await(context.Background(), "never-held")

	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
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

	pending, held := d.Pending("delivery-1")

	require.True(t, held)
	assert.Equal(t, "choice-1", pending.ChoiceID)
	assert.Equal(t, 15*time.Minute, pending.Wait, "the relay is told the clamped budget, not the declared one")
}

func TestHold_ARetriedDeliveryKeepsItsOriginalSlot(t *testing.T) {
	t.Parallel()

	d, first := fixture(time.Minute)

	again := d.Hold("delivery-1", answerdesk.Prompt{
		ChoiceID: "choice-1", ChatID: "chat-1", RunnerID: "runner-1",
	})

	assert.Same(t, first, again)
}

func TestHold_ReopeningAPromptReleasesTheRelayAlreadyHoldingIt(t *testing.T) {
	t.Parallel()

	d, stale := fixture(time.Minute)

	fresh := d.Hold("delivery-2", answerdesk.Prompt{
		ChoiceID: "choice-1", ChatID: "chat-1", RunnerID: "runner-1",
	})

	<-stale.Settled()
	held, ok := d.ByChoiceID("choice-1")
	require.True(t, ok)
	assert.Same(t, fresh, held)

	answer, err := d.Await(context.Background(), "delivery-1")
	require.NoError(t, err)
	assert.Empty(t, answer.Stdout, "the displaced relay prints nothing")
}

// One verdict, whatever the callers do. A second Resolve on a settled slot must
// not overwrite the answer a relay may already be printing.
func TestResolve_SettlesOnce(t *testing.T) {
	t.Parallel()

	d, slot := fixture(time.Minute)

	d.Resolve(slot, []byte("first"))
	d.Resolve(slot, []byte("second"))

	<-slot.Settled()
	answer, err := d.Await(context.Background(), "delivery-1")
	require.NoError(t, err)
	assert.Equal(t, "first", string(answer.Stdout))
}

func TestReleaseRunner_FreesOnlyThatRunnersRelays(t *testing.T) {
	t.Parallel()

	d, mine := fixture(time.Minute)
	theirs := d.Hold("delivery-2", answerdesk.Prompt{
		ChoiceID: "choice-2", ChatID: "chat-2", RunnerID: "runner-2",
	})

	released := d.ReleaseRunner(context.Background(), "runner-1")

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
	d.Resolve(slot, []byte("allow"))

	released := d.ReleaseRunner(context.Background(), "runner-1")

	assert.Empty(t, released, "a decided relay is not an abandoned one")
	_, waiting := d.Pending("delivery-1")
	assert.False(t, waiting, "but its slot is still freed")
}

func TestResolve_AVerdictIsRetainedForARelayThatHasNotAskedYet(t *testing.T) {
	t.Parallel()

	d, slot := fixture(time.Minute)

	d.Resolve(slot, []byte("allow"))

	_, waiting := d.Pending("delivery-1")
	require.True(t, waiting, "the decision waits on the desk for its relay")
	_, answerable := d.ByChoiceID("choice-1")
	assert.False(t, answerable, "while the prompt stops being answerable at once")

	answer, err := d.Await(context.Background(), "delivery-1")
	require.NoError(t, err)
	assert.Equal(t, "allow", string(answer.Stdout))
	_, stillWaiting := d.Pending("delivery-1")
	assert.False(t, stillWaiting, "and a claimed verdict is gone")
}

func TestResolve_AnUnclaimedVerdictDoesNotOutliveItsRetention(t *testing.T) {
	t.Parallel()

	d, slot := fixtureRetaining(time.Minute, -time.Second)

	d.Resolve(slot, []byte("allow"))

	answer, err := d.Await(context.Background(), "delivery-1")
	require.NoError(t, err)
	assert.Empty(t, answer.Stdout, "an expired verdict is not handed to a relay that came back late")

	_, waiting := d.Pending("delivery-1")
	assert.False(t, waiting, "and the reaper frees it on a desk nothing else ever touches")
}

func TestResolve_IgnoresASlotThatHasLeftTheDesk(t *testing.T) {
	t.Parallel()

	d, slot := fixture(time.Minute)
	d.Discard(slot)

	d.Resolve(slot, []byte("allow"))

	_, waiting := d.Pending("delivery-1")
	assert.False(t, waiting, "resolving a spent slot put it back on the desk")
}

func TestAbandon_ReportsTheQuestionToCloseWhenNobodyDecided(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Minute)

	d.Abandon(context.Background(), "delivery-1")

	_, waiting := d.Pending("delivery-1")
	assert.False(t, waiting)
}

func TestAbandon_ClosesNothingWhenTheVerdictWasAlreadyIn(t *testing.T) {
	t.Parallel()

	d, slot, ledger := withLedger(time.Minute)
	d.Resolve(slot, []byte("allow"))

	d.Abandon(context.Background(), "delivery-1")

	assert.Empty(t, ledger.all(), "the question was decided, not proceeded past")
}

func TestAbandon_IsSilentForADeliveryNobodyIsHolding(t *testing.T) {
	t.Parallel()

	ledger := &recordingLedger{}
	d := answerdesk.New(answerdesk.DefaultRetention, ledger)

	d.Abandon(context.Background(), "never-held")

	assert.Empty(t, ledger.all())
}

func TestAnswerableIDs_KeepsOnlyTheChoicesThisChatStillHolds(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Minute)
	d.Hold("delivery-2", answerdesk.Prompt{ChoiceID: "choice-2", ChatID: "chat-2", RunnerID: "runner-2"})

	ids := d.AnswerableIDs("chat-1", choices("choice-1", "choice-2", "choice-3"))

	assert.Equal(t, []string{"choice-1"}, ids,
		"another chat's held prompt and a prompt nobody holds are both unanswerable")
}

func TestAnswerableIDs_IsEmptyRatherThanNilForNoChoices(t *testing.T) {
	t.Parallel()

	d := answerdesk.New(answerdesk.DefaultRetention, nil)

	assert.Empty(t, d.AnswerableIDs("chat-1", nil))
}

// Four relays and one verdict. Exactly one prints it, and no interleaving loses
// it — the property the whole two-index, claim-once design exists for.
func TestAwait_AVerdictIsClaimedByExactlyOneOfManyRacingRelays(t *testing.T) {
	t.Parallel()

	for range 200 {
		d, slot := fixture(time.Minute)
		var printed atomic.Int64
		var relays sync.WaitGroup
		start := make(chan struct{})
		for range 4 {
			relays.Add(1)
			go func() {
				defer relays.Done()
				<-start
				answer, err := d.Await(context.Background(), "delivery-1")
				assert.NoError(t, err)
				if len(answer.Stdout) > 0 {
					printed.Add(1)
				}
			}()
		}
		relays.Add(1)
		go func() {
			defer relays.Done()
			<-start
			d.Resolve(slot, []byte("allow"))
		}()
		close(start)
		relays.Wait()

		require.Equal(t, int64(1), printed.Load(),
			"exactly one relay prints the decision, and no ordering loses it")
	}
}

// The desk owns the outcome write because it is the only thing that knows WHICH
// outcome a relay reached. These three pin the three verdicts apart.
func TestLedger_AbandonClosesTheQuestionAsProceeded(t *testing.T) {
	t.Parallel()

	d, _, ledger := withLedger(time.Minute)

	d.Abandon(context.Background(), "delivery-1")

	require.Equal(t, []resolution{{"chat-1", "choice-1", domain.ChoiceResolutionProceeded}}, ledger.all(),
		"an undecided prompt is being resolved by the provider's own UI")
}

func TestLedger_ReleaseRunnerClosesTheQuestionAsAbandoned(t *testing.T) {
	t.Parallel()

	d, _, ledger := withLedger(time.Minute)

	d.ReleaseRunner(context.Background(), "runner-1")

	require.Equal(t, []resolution{{"chat-1", "choice-1", domain.ChoiceResolutionAbandoned}}, ledger.all())
}

func TestLedger_AnsweredRelaysAreNotClosedAgainByADeadRunner(t *testing.T) {
	t.Parallel()

	d, slot, ledger := withLedger(time.Minute)
	d.Resolve(slot, []byte("allow"))

	d.ReleaseRunner(context.Background(), "runner-1")

	assert.Empty(t, ledger.all(), "a decided relay was answered, not abandoned")
}

// A ledger that refuses must not strand the relay: it has already been released,
// and refusing to admit that would leave the CLI blocked on a person who can no
// longer answer.
func TestLedger_AFailedWriteStillReleasesTheRelay(t *testing.T) {
	t.Parallel()

	ledger := &recordingLedger{err: errors.New("store is down")}
	d := answerdesk.New(answerdesk.DefaultRetention, ledger)
	slot := d.Hold("delivery-1", answerdesk.Prompt{ChoiceID: "choice-1", ChatID: "chat-1", RunnerID: "runner-1"})

	d.Abandon(context.Background(), "delivery-1")

	<-slot.Settled()
	_, waiting := d.Pending("delivery-1")
	assert.False(t, waiting)
}

// A desk with no ledger records nothing and refuses nothing. It is the shape a
// caller that only wants the parking behaviour gets.
func TestLedger_NilLedgerIsSilent(t *testing.T) {
	t.Parallel()

	d, _ := fixture(time.Minute)

	d.Abandon(context.Background(), "delivery-1")
	d.ReleaseRunner(context.Background(), "runner-1")
}
