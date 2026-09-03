package activity_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRegression_OpenChoiceSettlesBeforeAnswerChoiceCanRaceIt reproduces the
// live auto-approve race: OpenChoice immediately followed by AnswerChoice for
// the same choice, on a chat aggregate under concurrent unrelated write load
// (mirroring a live agent's own hook traffic landing on the same chat), must
// never see AnswerChoice reject the choice as "no longer pending" — that
// would mean OpenChoice returned before its write was visible to the very
// next command against the same aggregate.
func TestRegression_OpenChoiceSettlesBeforeAnswerChoiceCanRaceIt(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.repo.OpenTurn(f.ctx, activity.TurnInput{
		ChatID: chat, TurnID: "t1", ProviderID: "claude", Now: t0,
	}))

	const noiseWorkers = 4
	const noiseRoundsPerWorker = 15
	var noise sync.WaitGroup
	noise.Add(noiseWorkers)
	for w := range noiseWorkers {
		go func(worker int) {
			defer noise.Done()
			noiseRounds(f, worker, noiseRoundsPerWorker)
		}(w)
	}

	// Concurrent writers on the same aggregate (the noise goroutines above)
	// can legitimately exhaust a command's own OCC retry budget (asynx's
	// zero-backoff immediate retry scales poorly under contention on the
	// single-connection sqlite store) — that is a separate, pre-existing
	// backpressure concern, not the race under test. Skip those outcomes;
	// only a choice that OpenChoice itself reports as opened, answered with
	// something other than exhausted contention, is eligible to prove or
	// disprove the read-your-own-write property.
	const iterations = 60
	var opened, pendingRaces, otherErrs int
	for i := range iterations {
		id := fmt.Sprintf("c%d", i)
		if err := f.repo.OpenChoice(f.ctx, activity.ChoiceInput{
			ChatID: chat, ChoiceID: id, Kind: domain.ChoiceKindPermission, ToolName: "Bash",
			Options: []domain.ActivityChoiceOption{
				{ID: "allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
			},
			Now: t0,
		}); err != nil {
			continue
		}
		opened++

		answerErr := f.repo.AnswerChoice(f.ctx, chat, id, []string{"allow"}, true, t0)
		switch classifyAnswerErr(answerErr) {
		case answerRace:
			pendingRaces++
		case answerOther:
			otherErrs++
			t.Logf("unrelated AnswerChoice error: %v", answerErr)
		}
	}

	noise.Wait()

	require.Greater(t, opened, iterations/2,
		"the harness must actually exercise enough concurrently-opened choices to be a meaningful test")
	assert.Zero(t, otherErrs, "no unrelated AnswerChoice errors expected")
	assert.Zero(t, pendingRaces,
		"OpenChoice must durably settle before returning: %d/%d immediate answers raced it", pendingRaces, opened)
}

type answerOutcome int

const (
	answerOK answerOutcome = iota
	answerRace
	answerContended
	answerOther
)

func classifyAnswerErr(err error) answerOutcome {
	if err == nil {
		return answerOK
	}
	if strings.Contains(err.Error(), "no longer pending") {
		return answerRace
	}
	if strings.Contains(err.Error(), "pipeline failed") {
		return answerContended
	}
	return answerOther
}

// noiseRounds mimics a live agent's own hook traffic landing on the same chat
// aggregate: real writes, from a real goroutine, with no coordination with
// the Open/Answer pair under test. Bounded so the whole test finishes in
// reasonable time under -race: asynx retries a version conflict immediately
// with no backoff, so throughput on one aggregate degrades sharply as
// concurrent writers pile up — a pre-existing scalability property, and
// orthogonal to the read-your-own-write property this test targets.
func noiseRounds(f fixture, worker, rounds int) {
	for i := range rounds {
		id := fmt.Sprintf("noise-%d-%d", worker, i)
		_ = f.repo.InvokeTool(f.ctx, activity.ToolInput{
			ChatID: chat, ToolID: id, Name: "Read", Now: t0,
		})
		_ = f.repo.CompleteTool(f.ctx, activity.ToolResultInput{
			ChatID: chat, ToolID: id, Status: domain.ToolStatusOK, Now: t0,
		})
	}
}
