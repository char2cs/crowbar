package ingress

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
)

func TestDeadLetter_RecordsTheReasonAndTheRunner(t *testing.T) {
	f := newFixture()
	h := hook{runnerID: "runner-1", provider: "p", event: "tool_pre", raw: []byte(`{"x":1}`)}

	f.ingress.deadLetter(t.Context(), h, "chat-1", reasonUnknownRunner)

	require.Len(t, f.ledger.letters, 1)
	assert.Equal(t, ledgerstore.DeadLetter{
		ChatID: "chat-1", RunnerID: "runner-1", Provider: "p", Event: "tool_pre",
		Reason: reasonUnknownRunner, Payload: []byte(`{"x":1}`),
	}, f.ledger.letters[0])
}

func TestDeadLetter_AFailedWriteIsNotTheSendersProblem(t *testing.T) {
	f := newFixture()
	f.ledger.failLetters = errors.New("disk full")

	f.ingress.deadLetter(t.Context(), hook{runnerID: "runner-1"}, "", reasonUnknownRunner)

	assert.Empty(t, f.ledger.letters)
}
