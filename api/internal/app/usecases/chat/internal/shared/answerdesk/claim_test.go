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
	ledgerrows "github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestAwait_TheBudgetExpiringPrintsNothingAndClosesItsQuestion(t *testing.T) {
	t.Parallel()
	d, _, ledger := withLedger(time.Millisecond)

	answer, err := d.Await(context.Background(), "choice-1", domain.ChoiceResolutionRefused)
	d.Abandon(context.Background(), "choice-1")

	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
	waiting := d.Holds("choice-1")
	assert.False(t, waiting, "an expired wait must not leave a slot an answer could reach")
	assert.Equal(t, []resolution{{"chat-1", "choice-1", domain.ChoiceResolutionRefused}}, ledger.all())
}

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
	d := answerdesk.New(answerdesk.DefaultRetention, &recordingLedger{})

	answer, err := d.Await(context.Background(), "never-held", proceeded)

	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
}

func TestAwait_AVerdictIsClaimedByExactlyOneOfManyRacingRelays(t *testing.T) {
	t.Parallel()

	for range 200 {
		d, slot := fixture()
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

	d, slot := fixture()
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

func TestAnswer_SettlesOnce(t *testing.T) {
	t.Parallel()

	d, slot := fixture()

	require.NoError(t, decide(d, slot, "first"))
	require.ErrorIs(t, decide(d, slot, "second"), answerdesk.ErrNotWaiting)

	<-slot.Settled()
	answer, err := d.Await(context.Background(), "choice-1", proceeded)
	require.NoError(t, err)
	assert.Equal(t, "first", string(answer.Stdout))
}

func TestAnswer_AVerdictIsRetainedForARelayThatHasNotAskedYet(t *testing.T) {
	t.Parallel()

	d, slot := fixture()

	require.NoError(t, decide(d, slot, "allow"))

	waiting := d.Holds("choice-1")
	require.True(t, waiting, "the decision waits on the desk for its relay")
	_, answerable := d.ByChoiceID("choice-1")
	assert.False(t, answerable, "while the prompt stops being answerable at once")

	answer, err := d.Await(context.Background(), "choice-1", proceeded)
	require.NoError(t, err)
	assert.Equal(t, "allow", string(answer.Stdout))
	stillWaiting := d.Holds("choice-1")
	assert.False(t, stillWaiting, "and a claimed verdict is gone")
}

func TestAnswer_AnUnclaimedVerdictDoesNotOutliveItsRetention(t *testing.T) {
	t.Parallel()

	d, slot := fixtureRetaining(-time.Second)

	require.NoError(t, decide(d, slot, "allow"))

	answer, err := d.Await(context.Background(), "choice-1", proceeded)
	require.NoError(t, err)
	assert.Empty(t, answer.Stdout, "an expired verdict is not handed to a relay that came back late")

	waiting := d.Holds("choice-1")
	assert.False(t, waiting, "and the reaper frees it on a desk nothing else ever touches")
}

func TestAnswer_IgnoresASlotThatHasLeftTheDesk(t *testing.T) {
	t.Parallel()

	d, slot := fixture()
	d.Discard(slot)

	require.ErrorIs(t, decide(d, slot, "allow"), answerdesk.ErrNotWaiting)

	waiting := d.Holds("choice-1")
	assert.False(t, waiting, "resolving a spent slot put it back on the desk")
}

func relay(ctx context.Context, d *answerdesk.Desk) <-chan string {
	printed := make(chan string, 1)
	go func() {
		answer, _ := d.Await(ctx, "choice-1", proceeded)
		printed <- string(answer.Stdout)
	}()
	return printed
}

func TestAnswer_ARelayReleasedFirstRefusesTheAnswerAndRecordsNothing(t *testing.T) {
	t.Parallel()
	d, slot, ledger := withLedger(time.Millisecond)
	printed := relay(context.Background(), d)
	<-slot.Settled()

	asked := false
	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(context.Context, func() bool) (bool, error) {
		asked = true
		return true, nil
	})

	require.ErrorIs(t, err, problem.ErrConflict, "a relay that stopped waiting cannot be answered")
	require.ErrorIs(t, err, answerdesk.ErrNotWaiting)
	assert.False(t, answered)
	assert.False(t, asked, "an answer its CLI never got is never recorded")
	assert.Empty(t, <-printed)
	assert.Equal(t, []resolution{{"chat-1", "choice-1", proceeded}}, ledger.all())
}

func TestRegression_ARelayHangingUpBeforeItsAnswerIsWrittenLeavesItProceeded(t *testing.T) {
	t.Parallel()
	d, slot, ledger := withLedger(time.Minute)

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(ctx context.Context, waiting func() bool) (bool, error) {
		d.Abandon(ctx, "choice-1")
		return waiting() && ledger.answer(), nil
	})

	require.ErrorIs(t, err, answerdesk.ErrNotWaiting)
	assert.False(t, answered)
	assert.Equal(t, []resolution{{"chat-1", "choice-1", proceeded}}, ledger.all(), "exactly one outcome, the first")
}

func TestAnswer_ARelayHangingUpAfterItsAnswerIsWrittenLeavesItAnswered(t *testing.T) {
	t.Parallel()
	d, slot, ledger := withLedger(time.Minute)

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(ctx context.Context, waiting func() bool) (bool, error) {
		first := waiting() && ledger.answer()
		d.Abandon(ctx, "choice-1")
		return first, nil
	})

	require.NoError(t, err)
	assert.True(t, answered)
	assert.Equal(t, []resolution{{"chat-1", "choice-1", ledgerrows.ResolutionAnswered}}, ledger.all())
}

func TestRegression_AnAnswerItsRunnersExitReleasedFirstRecordsNothing(t *testing.T) {
	t.Parallel()
	d, slot, ledger := withLedger(time.Minute)

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(_ context.Context, waiting func() bool) (bool, error) {
		d.ReleaseRunner("runner-1")
		return waiting() && ledger.answer(), nil
	})

	require.ErrorIs(t, err, answerdesk.ErrNotWaiting)
	assert.False(t, answered)
	assert.Empty(t, ledger.all())
}

func TestRegression_AnAskAskedAgainMidRecordHandsTheAnswerToItsNewRelay(t *testing.T) {
	t.Parallel()
	budget := make(chan time.Time)
	d, slot, ledger := clocked(budget)
	var printed <-chan string

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(ctx context.Context, waiting func() bool) (bool, error) {
		d.Hold(slot.Prompt)
		printed = relay(ctx, d)
		budget <- time.Now()
		return waiting() && ledger.answer(), nil
	})

	require.NoError(t, err)
	assert.True(t, answered)
	assert.Equal(t, "allow", <-printed, "the relay now holding the ask is handed its answer")
	assert.Equal(t, []resolution{{"chat-1", "choice-1", ledgerrows.ResolutionAnswered}}, ledger.all())
}

func TestRegression_AnAnswerLookedUpBeforeAReAskReachesItsNewRelay(t *testing.T) {
	t.Parallel()
	d, looked := fixture()
	d.Hold(looked.Prompt)
	printed := relay(context.Background(), d)

	answered, err := d.Answer(context.Background(), looked, []byte("allow"), recorded)

	require.NoError(t, err)
	assert.True(t, answered)
	assert.Equal(t, "allow", <-printed)
}

func TestAwait_ADisplacedRelayPrintsNothing(t *testing.T) {
	t.Parallel()
	parked := make(chan struct{}, 2)
	d, slot, _ := withLedger(time.Minute, answerdesk.WithBudgetClock(func(time.Duration) <-chan time.Time {
		parked <- struct{}{}
		return nil
	}))
	first := relay(context.Background(), d)
	<-parked

	fresh := d.Hold(slot.Prompt)

	assert.Empty(t, <-first, "the displaced relay prints nothing")
	second := relay(context.Background(), d)
	<-parked
	require.NoError(t, decide(d, fresh, "allow"))
	assert.Equal(t, "allow", <-second)
}

func clocked(budget chan time.Time) (*answerdesk.Desk, *answerdesk.Slot, *recordingLedger) {
	return withLedger(time.Minute, answerdesk.WithBudgetClock(func(time.Duration) <-chan time.Time { return budget }))
}

func TestAnswer_ABudgetRunningOutMidRecordWaitsForTheAnswer(t *testing.T) {
	t.Parallel()
	budget := make(chan time.Time)
	d, slot, ledger := clocked(budget)
	printed := relay(context.Background(), d)

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(context.Context, func() bool) (bool, error) {
		budget <- time.Now()
		return true, nil
	})

	require.NoError(t, err)
	assert.True(t, answered)
	assert.Equal(t, "allow", <-printed, "the relay is handed the answer that claimed it")
	assert.Empty(t, ledger.all(), "the expiry lost the relay and records nothing")
}

func TestAnswer_AFailedRecordHandsAnExpiredRelayBackToItsBudget(t *testing.T) {
	t.Parallel()
	budget := make(chan time.Time)
	d, slot, ledger := clocked(budget)
	printed := relay(context.Background(), d)

	answered, err := d.Answer(context.Background(), slot, []byte("allow"), func(context.Context, func() bool) (bool, error) {
		budget <- time.Now()
		return false, errors.New("log is full")
	})

	require.Error(t, err)
	assert.False(t, answered)
	assert.Empty(t, <-printed)
	assert.Equal(t, []resolution{{"chat-1", "choice-1", proceeded}}, ledger.all())
}
