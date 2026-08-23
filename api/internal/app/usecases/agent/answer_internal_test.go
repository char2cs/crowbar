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

func deskFixture(wait time.Duration) (*answerUsecase, *answerSlot) {
	u := &answerUsecase{answers: newAnswerDesk()}
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

func TestAwaitAnswer_TheBudgetExpiringPrintsNothingAndFreesTheSlot(t *testing.T) {
	u, _ := deskFixture(time.Millisecond)

	answer, err := u.AwaitAnswer(context.Background(), "delivery-1")

	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
	_, waiting := u.answers.byDeliveryID("delivery-1")
	assert.False(t, waiting, "an expired wait must not leave a slot an answer could reach")
}

func TestAnswerWait_ClampsAnAbsentOrAbsurdBudget(t *testing.T) {
	assert.Equal(t, answerWaitFallback, answerWait(0))
	assert.Equal(t, answerWaitFallback, answerWait(-time.Second))
	assert.Equal(t, maxAnswerWait, answerWait(time.Hour))
	assert.Equal(t, 30*time.Second, answerWait(30*time.Second))
}

func TestAnswerDesk_ARetriedDeliveryKeepsItsOriginalSlot(t *testing.T) {
	u, first := deskFixture(time.Minute)

	again := u.answers.open("delivery-1", &answerSlot{
		choiceID: "choice-1", chatID: "chat-1", runnerID: "runner-1",
		done: make(chan struct{}),
	})

	assert.Same(t, first, again)
}

func TestAnswerDesk_ReopeningAPromptReleasesTheRelayAlreadyHoldingIt(t *testing.T) {
	u, stale := deskFixture(time.Minute)

	fresh := u.answers.open("delivery-2", &answerSlot{
		choiceID: "choice-1", chatID: "chat-1", runnerID: "runner-1",
		done: make(chan struct{}),
	})

	<-stale.done
	assert.Empty(t, stale.stdout)
	held, ok := u.answers.byChoiceID("choice-1")
	require.True(t, ok)
	assert.Same(t, fresh, held)
}

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
