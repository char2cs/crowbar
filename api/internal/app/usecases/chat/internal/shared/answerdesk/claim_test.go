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

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A relay whose budget runs out prints nothing, and the desk closes its question
// once, as what the relay's channel hands its CLI then.
func TestAwait_TheBudgetExpiringPrintsNothingAndClosesItsQuestion(t *testing.T) {
	t.Parallel()
	d, _, ledger := withLedger(time.Millisecond)

	answer, err := d.Await(context.Background(), "choice-1", domain.ChoiceResolutionRefused)
	d.Abandon(context.Background(), "choice-1")

	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
	_, waiting := d.Pending("choice-1")
	assert.False(t, waiting, "an expired wait must not leave a slot an answer could reach")
	assert.Equal(t, []resolution{{"chat-1", "choice-1", domain.ChoiceResolutionRefused}}, ledger.all())
}

// A relay that stops waiting leaves its ask to be retired: abandoned, its
// question is closed as proceeded past, as the CLI now asks it itself.
func TestAwait_ACancelledRelayLeavesItsAskToItsAbandon(t *testing.T) {
	t.Parallel()
	d, _, ledger := withLedger(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	answer, err := d.Await(ctx, "choice-1", proceeded)
	d.Abandon(context.Background(), "choice-1")

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, answer.Stdout)
	assert.Equal(t, []resolution{{"chat-1", "choice-1", domain.ChoiceResolutionProceeded}}, ledger.all())
}

func TestAwait_AnAskNobodyIsHoldingReturnsNothing(t *testing.T) {
	t.Parallel()
	d := answerdesk.New(answerdesk.DefaultRetention, nil)

	answer, err := d.Await(context.Background(), "never-held", proceeded)

	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
}

// Four relays and one verdict. Exactly one prints it, and no interleaving loses
// it — the property the claim-once design exists for.
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
				answer, err := d.Await(context.Background(), "choice-1", proceeded)
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
			assert.NoError(t, decide(d, slot, "allow"))
		}()
		close(start)
		relays.Wait()

		require.Equal(t, int64(1), printed.Load(),
			"exactly one relay prints the decision, and no ordering loses it")
	}
}

func TestAnswer_ReleasesTheRelayHeldOnThatAsk(t *testing.T) {
	t.Parallel()

	d, slot := fixture(time.Minute)
	other := d.Hold(answerdesk.Prompt{ChoiceID: "choice-2", ChatID: "chat-1", RunnerID: "runner-1"})
	relayed := make(chan answerdesk.HookAnswer, 1)
	go func() {
		answer, err := d.Await(context.Background(), "choice-1", proceeded)
		assert.NoError(t, err)
		relayed <- answer
	}()

	require.NoError(t, decide(d, slot, "allow"))

	assert.Equal(t, "allow", string((<-relayed).Stdout))
	held, ok := d.ByChoiceID("choice-2")
	require.True(t, ok, "the other ask is still open")
	assert.Same(t, other, held)
}

// One verdict, whatever the callers do. A second answer to a settled slot must
// not overwrite the answer a relay may already be printing.
func TestAnswer_SettlesOnce(t *testing.T) {
	t.Parallel()

	d, slot := fixture(time.Minute)

	require.NoError(t, decide(d, slot, "first"))
	require.ErrorIs(t, decide(d, slot, "second"), answerdesk.ErrNotWaiting)

	<-slot.Settled()
	answer, err := d.Await(context.Background(), "choice-1", proceeded)
	require.NoError(t, err)
	assert.Equal(t, "first", string(answer.Stdout))
}

func TestAnswer_AVerdictIsRetainedForARelayThatHasNotAskedYet(t *testing.T) {
	t.Parallel()

	d, slot := fixture(time.Minute)

	require.NoError(t, decide(d, slot, "allow"))

	_, waiting := d.Pending("choice-1")
	require.True(t, waiting, "the decision waits on the desk for its relay")
	_, answerable := d.ByChoiceID("choice-1")
	assert.False(t, answerable, "while the prompt stops being answerable at once")

	answer, err := d.Await(context.Background(), "choice-1", proceeded)
	require.NoError(t, err)
	assert.Equal(t, "allow", string(answer.Stdout))
	_, stillWaiting := d.Pending("choice-1")
	assert.False(t, stillWaiting, "and a claimed verdict is gone")
}

func TestAnswer_AnUnclaimedVerdictDoesNotOutliveItsRetention(t *testing.T) {
	t.Parallel()

	d, slot := fixtureRetaining(time.Minute, -time.Second)

	require.NoError(t, decide(d, slot, "allow"))

	answer, err := d.Await(context.Background(), "choice-1", proceeded)
	require.NoError(t, err)
	assert.Empty(t, answer.Stdout, "an expired verdict is not handed to a relay that came back late")

	_, waiting := d.Pending("choice-1")
	assert.False(t, waiting, "and the reaper frees it on a desk nothing else ever touches")
}

func TestAnswer_IgnoresASlotThatHasLeftTheDesk(t *testing.T) {
	t.Parallel()

	d, slot := fixture(time.Minute)
	d.Discard(slot)

	require.ErrorIs(t, decide(d, slot, "allow"), answerdesk.ErrNotWaiting)

	_, waiting := d.Pending("choice-1")
	assert.False(t, waiting, "resolving a spent slot put it back on the desk")
}

// relay parks a relay on choice-1 and reports what it prints.
func relay(d *answerdesk.Desk) <-chan string {
	printed := make(chan string, 1)
	go func() {
		answer, _ := d.Await(context.Background(), "choice-1", proceeded)
		printed <- string(answer.Stdout)
	}()
	return printed
}

// An answer looked up before its relay's budget ran out comes too late: it is
// refused and records nothing, and the relay's own outcome stands.
func TestAnswer_ARelayReleasedFirstRefusesTheAnswerAndRecordsNothing(t *testing.T) {
	t.Parallel()
	d, slot, ledger := withLedger(time.Millisecond)
	printed := relay(d)
	<-slot.Settled()

	asked := false
	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(context.Context) (bool, error) {
		asked = true
		return true, nil
	})

	require.ErrorIs(t, err, apperr.ErrConflict, "a relay that stopped waiting cannot be answered")
	require.ErrorIs(t, err, answerdesk.ErrNotWaiting)
	assert.False(t, answered)
	assert.False(t, asked, "an answer its CLI never got is never recorded")
	assert.Empty(t, <-printed)
	assert.Equal(t, []resolution{{"chat-1", "choice-1", proceeded}}, ledger.all())
}

// The answer being recorded owns its relay: the relay hanging up meanwhile
// writes no outcome of its own.
func TestAnswer_ARelayHangingUpMidRecordLeavesTheOutcomeToTheAnswer(t *testing.T) {
	t.Parallel()
	d, slot, ledger := withLedger(time.Minute)

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(ctx context.Context) (bool, error) {
		d.Abandon(ctx, "choice-1")
		return true, nil
	})

	require.NoError(t, err)
	assert.True(t, answered)
	assert.Empty(t, ledger.all(), "only the answer that claimed the relay records its outcome")
}

// clocked is a desk whose relay budget runs out when the test sends on budget.
func clocked(budget chan time.Time) (*answerdesk.Desk, *answerdesk.Slot, *recordingLedger) {
	return withLedger(time.Minute, answerdesk.WithBudgetClock(func(time.Duration) <-chan time.Time { return budget }))
}

// A budget that runs out while an answer is being recorded waits for it: the
// relay prints the answer, and the expiry writes nothing.
func TestAnswer_ABudgetRunningOutMidRecordWaitsForTheAnswer(t *testing.T) {
	t.Parallel()
	budget := make(chan time.Time)
	d, slot, ledger := clocked(budget)
	printed := relay(d)

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(context.Context) (bool, error) {
		budget <- time.Now()
		return true, nil
	})

	require.NoError(t, err)
	assert.True(t, answered)
	assert.Equal(t, "allow", <-printed, "the relay is handed the answer that claimed it")
	assert.Empty(t, ledger.all(), "the expiry lost the relay and records nothing")
}

// An answer that could not be recorded hands its relay back, and a budget that
// ran out meanwhile then releases it and records the outcome it delivered.
func TestAnswer_AFailedRecordHandsAnExpiredRelayBackToItsBudget(t *testing.T) {
	t.Parallel()
	budget := make(chan time.Time)
	d, slot, ledger := clocked(budget)
	printed := relay(d)

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(context.Context) (bool, error) {
		budget <- time.Now()
		return false, errors.New("log is full")
	})

	require.Error(t, err)
	assert.False(t, answered)
	assert.Empty(t, <-printed)
	assert.Equal(t, []resolution{{"chat-1", "choice-1", proceeded}}, ledger.all())
}
