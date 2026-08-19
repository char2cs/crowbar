package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
)

// TestAwaitPrompt_ForwardsTheRunnerAndCredentialAndReturnsTheMessage pins the
// whole contract of the one Crowbar callback that READS: the runner comes from the
// path, the credential from the body, and the user's words come back under the
// standard envelope.
func TestAwaitPrompt_ForwardsTheRunnerAndCredentialAndReturnsTheMessage(t *testing.T) {
	uc := &fakeAgentUsecase{promptAwaitPrompt: "say only ACK", promptAwaitFound: true}
	h, ctx, rec := awaitContext(t, uc, `{"token":"TOK","waitMs":60000}`)

	h.AwaitPrompt(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.promptAwaitCalls, 1)
	assert.Equal(t, "seg-1", uc.promptAwaitCalls[0].runnerID)
	assert.Equal(t, "TOK", uc.promptAwaitCalls[0].token)
	assert.Equal(t, int64(60000), uc.promptAwaitCalls[0].waitMS)
	assert.JSONEq(t, `{"success":true,"data":{"prompt":"say only ACK"}}`, rec.Body.String())
}

// TestAwaitPrompt_AcknowledgesOnlyAfterTheBodyIsWritten is the ordering the
// delivery record depends on. The daemon hands a message over exactly once, so ack
// is the only evidence the handover completed — taken before the write, it would
// record a delivery that had not happened.
func TestAwaitPrompt_AcknowledgesOnlyAfterTheBodyIsWritten(t *testing.T) {
	uc := &fakeAgentUsecase{promptAwaitPrompt: "say only ACK", promptAwaitFound: true}
	h, ctx, rec := awaitContext(t, uc, `{"token":"TOK","waitMs":1000}`)

	h.AwaitPrompt(ctx)

	assert.True(t, uc.promptAwaitAcked, "a delivered message must be acknowledged")
	assert.Contains(t, rec.Body.String(), "say only ACK")
}

// TestAwaitPrompt_NothingToCollectIs204AndNeverAcknowledged: the steady state. A
// collector spends its whole life here and asks again, and nothing was handed over
// to acknowledge.
func TestAwaitPrompt_NothingToCollectIs204AndNeverAcknowledged(t *testing.T) {
	uc := &fakeAgentUsecase{promptAwaitFound: false}
	h, ctx, rec := awaitContext(t, uc, `{"token":"TOK","waitMs":1000}`)

	h.AwaitPrompt(ctx)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.False(t, uc.promptAwaitAcked)
}

// TestAwaitPrompt_ARejectedCredentialIs403 keeps a bad token out of the 500 bucket:
// the daemon is healthy and the request is simply not authorised. 403 rather than
// 401 because there is no challenge to issue — the credential is minted at spawn.
func TestAwaitPrompt_ARejectedCredentialIs403(t *testing.T) {
	uc := &fakeAgentUsecase{promptAwaitErr: fmt.Errorf("bad token: %w", agenttools.ErrUnauthorized)}
	h, ctx, rec := awaitContext(t, uc, `{"token":"WRONG","waitMs":1000}`)

	h.AwaitPrompt(ctx)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.False(t, uc.promptAwaitAcked)
}

// TestAwaitPrompt_AnErrorNeverCarriesAMessage: the collector prints whatever comes
// back on a 2xx, so an error path that leaked a body would put an error string
// into somebody's conversation as though they had typed it.
func TestAwaitPrompt_AnErrorNeverCarriesAMessage(t *testing.T) {
	uc := &fakeAgentUsecase{promptAwaitErr: errors.New("the runner is gone")}
	h, ctx, rec := awaitContext(t, uc, `{"token":"TOK","waitMs":1000}`)

	h.AwaitPrompt(ctx)

	assert.GreaterOrEqual(t, rec.Code, http.StatusBadRequest)
	assert.NotContains(t, rec.Body.String(), `"prompt"`)
}

func TestAwaitPrompt_MalformedBodyIs400(t *testing.T) {
	uc := &fakeAgentUsecase{}
	h, ctx, rec := awaitContext(t, uc, `{`)

	h.AwaitPrompt(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.promptAwaitCalls)
}

// awaitContext builds the handler and a request against the runner-keyed route,
// exactly as the router mounts it: the runner in the path, the credential in the
// body.
func awaitContext(
	t *testing.T,
	uc *fakeAgentUsecase,
	body string,
) (*handlers.Handlers, *gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	h := newChatHandlers(uc)
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/agent/runners/seg-1/prompt-await", []byte(body))
	ctx.Params = gin.Params{{Key: "segid", Value: "seg-1"}}
	return h, ctx, rec
}
