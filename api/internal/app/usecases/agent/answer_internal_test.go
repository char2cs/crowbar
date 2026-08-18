package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The desk's own mechanics are reached directly here because two of its
// properties cannot be driven through a real provider: the descriptor's declared
// budget is minutes long, and the clamps exist for descriptors nobody has
// authored. A test that waited out a real budget would be a test that sleeps.

func deskFixture(wait time.Duration) (*Usecase, *answerSlot) {
	u := &Usecase{answers: newAnswerDesk()}
	slot := u.answers.open("delivery-1", &answerSlot{
		choiceID: "choice-1",
		chatID:   "chat-1",
		runnerID: "runner-1",
		event:    engineagents.HookPermission,
		keys:     engineagents.AnswerCapability{Wait: wait, Keys: []string{"allow"}},
		done:     make(chan struct{}),
	})
	return u, slot
}

// The budget expiring is the relay exiting under CROWBAR's control rather than
// being killed mid-write by the provider's own hook timeout. It prints nothing,
// and the CLI's dialog is what the human answers.
func TestAwaitAnswer_TheBudgetExpiringPrintsNothingAndFreesTheSlot(t *testing.T) {
	u, _ := deskFixture(time.Millisecond)

	answer, err := u.AwaitAnswer(context.Background(), "delivery-1")

	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
	_, waiting := u.answers.byDeliveryID("delivery-1")
	assert.False(t, waiting, "an expired wait must not leave a slot an answer could reach")
}

// A descriptor that declares an answer channel with no budget is mis-authored,
// not a request for an unbounded wait: a slot with no deadline is a relay that
// never exits.
func TestAnswerWait_ClampsAnAbsentOrAbsurdBudget(t *testing.T) {
	assert.Equal(t, answerWaitFallback, answerWait(0))
	assert.Equal(t, answerWaitFallback, answerWait(-time.Second))
	assert.Equal(t, maxAnswerWait, answerWait(time.Hour))
	assert.Equal(t, 30*time.Second, answerWait(30*time.Second))
}

// The relay retries ONE delivery id until the daemon acknowledges it, so a retry
// must find the prompt it already opened rather than a second copy of it.
func TestAnswerDesk_ARetriedDeliveryKeepsItsOriginalSlot(t *testing.T) {
	u, first := deskFixture(time.Minute)

	again := u.answers.open("delivery-1", &answerSlot{
		choiceID: "choice-1", chatID: "chat-1", runnerID: "runner-1",
		done: make(chan struct{}),
	})

	assert.Same(t, first, again)
}

// Two relays waiting on one prompt could each be handed the same verdict, and the
// provider would be told twice. The older one is released first.
func TestAnswerDesk_ReopeningAPromptReleasesTheRelayAlreadyHoldingIt(t *testing.T) {
	u, stale := deskFixture(time.Minute)

	fresh := u.answers.open("delivery-2", &answerSlot{
		choiceID: "choice-1", chatID: "chat-1", runnerID: "runner-1",
		done: make(chan struct{}),
	})

	<-stale.done // released, with no verdict
	assert.Empty(t, stale.stdout)
	held, ok := u.answers.byChoiceID("choice-1")
	require.True(t, ok)
	assert.Same(t, fresh, held)
}

// A prompt answered in Crowbar at the instant its CLI dies must wake exactly one
// waiter with exactly one verdict.
func TestAnswerSlot_SettlesOnce(t *testing.T) {
	slot := &answerSlot{done: make(chan struct{})}

	slot.settle([]byte("first"))
	slot.settle([]byte("second"))

	<-slot.done
	assert.Equal(t, "first", string(slot.stdout))
}

func TestAnswerDesk_ReleaseRunnerFreesOnlyThatRunnersRelays(t *testing.T) {
	u, mine := deskFixture(time.Minute)
	theirs := u.answers.open("delivery-2", &answerSlot{
		choiceID: "choice-2", chatID: "chat-2", runnerID: "runner-2",
		done: make(chan struct{}),
	})

	released := u.answers.releaseRunner("runner-1")

	require.Len(t, released, 1)
	assert.Same(t, mine, released[0])
	<-mine.done
	_, stillHeld := u.answers.byChoiceID("choice-2")
	assert.True(t, stillHeld, "another runner's relay must not be collateral")
	select {
	case <-theirs.done:
		t.Fatal("another runner's relay was released")
	default:
	}
}

// The verdict is RETAINED, not merely settled: nothing has to be parked on the
// slot when the decision lands for a relay to collect it afterwards. That gap is
// two HTTP round trips wide in production.
func TestAnswerDesk_AVerdictIsRetainedForARelayThatHasNotAskedYet(t *testing.T) {
	u, slot := deskFixture(time.Minute)

	u.answers.resolve(slot, []byte("allow"))

	held, waiting := u.answers.byDeliveryID("delivery-1")
	require.True(t, waiting, "the decision waits on the desk for its relay")
	assert.Same(t, slot, held)
	_, answerable := u.answers.byChoiceID("choice-1")
	assert.False(t, answerable, "while the prompt stops being answerable at once")

	stdout, claimed := u.answers.claim(slot)
	require.True(t, claimed)
	assert.Equal(t, "allow", string(stdout))
	_, stillWaiting := u.answers.byDeliveryID("delivery-1")
	assert.False(t, stillWaiting, "and a claimed verdict is gone")
}

// A verdict nobody claims must not leak. A relay can die between the ack and the
// await and never come back, so retention is bounded on the desk's own clock as
// well as swept by the death of its runner.
func TestAnswerDesk_AnUnclaimedVerdictDoesNotOutliveItsRetention(t *testing.T) {
	u, slot := deskFixture(time.Minute)
	u.answers.retention = -time.Second

	u.answers.resolve(slot, []byte("allow"))

	_, claimed := u.answers.claim(slot)
	assert.False(t, claimed, "an expired verdict is not handed to a relay that came back late")

	u.answers.dropExpired()
	_, waiting := u.answers.byDeliveryID("delivery-1")
	assert.False(t, waiting, "and the reaper frees it on a desk nothing else ever touches")
}

// The claim must be ATOMIC, not merely usually-once. Several relays polling one
// delivery id while the answer lands between them is the production shape of this
// race, and a verdict handed out twice is a gated tool the provider runs twice.
//
// It is a REAL race, and a two-sided one: one barrier releases four waiters and
// the answer together. Instrumented over 200 iterations under -race, 92 claimed a
// verdict that was already on the desk — the production window — and 108 claimed
// one after parking on it.
func TestAnswerDesk_AVerdictIsClaimedByExactlyOneOfManyRacingRelays(t *testing.T) {
	for range 200 {
		u, slot := deskFixture(time.Minute)
		var printed atomic.Int64
		var relays sync.WaitGroup
		start := make(chan struct{})
		for range 4 {
			relays.Add(1)
			go func() {
				defer relays.Done()
				<-start
				answer, err := u.AwaitAnswer(context.Background(), "delivery-1")
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
			u.answers.resolve(slot, []byte("allow"))
		}()
		close(start)
		relays.Wait()

		require.Equal(t, int64(1), printed.Load(),
			"exactly one relay prints the decision, and no ordering loses it")
	}
}
