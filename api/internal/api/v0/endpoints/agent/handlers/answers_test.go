package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// answerRequest builds the POST a chat client makes when a human presses one of a
// prompt's buttons.
func answerRequest(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, rec := newTestContext(t, http.MethodPost,
		"/v0/projects/p/repos/r/workspaces/ws-1/agent/chats/chat-1/choices/c1/answer",
		[]byte(body))
	ctx.Params = gin.Params{
		{Key: "wsId", Value: "ws-1"},
		{Key: "id", Value: "chat-1"},
		{Key: "choiceId", Value: "c1"},
	}
	return ctx, rec
}

func TestAnswerChoice_ForwardsThePicksVerbatim(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := answerRequest(t,
		`{"optionIds":["answer-1"],"reason":"because","content":{"choice":"B"}}`)

	newChatHandlers(inWorkspace(uc)).AnswerChoice(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.answerCalls, 1)
	assert.Equal(t, "chat-1", uc.answerCalls[0].chatID)
	assert.Equal(t, "c1", uc.answerCalls[0].choiceID)
	assert.Equal(t, []string{"answer-1"}, uc.answerCalls[0].optionIDs)
	assert.Equal(t, "because", uc.answerCalls[0].reason)
	assert.JSONEq(t, `{"choice":"B"}`, uc.answerCalls[0].content,
		"an elicitation's form is passed through uninterpreted")
}

// No relay is holding the gate any more: the CLI is asking at its own terminal by
// now, so reporting success would be a lie a client cannot recover from.
func TestAnswerChoice_APromptNobodyIsWaitingOnIsAConflict(t *testing.T) {
	uc := &fakeAgentUsecase{answerErr: apperr.ErrConflict}
	ctx, rec := answerRequest(t, `{"optionIds":["allow"]}`)

	newChatHandlers(inWorkspace(uc)).AnswerChoice(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestAnswerChoice_RejectsAnEmptyOrMalformedDecision(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "not json", body: "{"},
		{name: "no options", body: `{"optionIds":[]}`},
		{name: "missing options", body: `{"reason":"x"}`},
		{name: "oversize reason", body: `{"optionIds":["allow"],"reason":"` +
			strings.Repeat("x", 5<<10) + `"}`},
		{name: "oversize content", body: `{"optionIds":["allow"],"content":{"x":"` +
			strings.Repeat("y", 70<<10) + `"}}`},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uc := &fakeAgentUsecase{}
			ctx, rec := answerRequest(t, tc.body)

			newChatHandlers(inWorkspace(uc)).AnswerChoice(ctx)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, uc.answerCalls, "a rejected decision must not reach the usecase")
		})
	}
}

// The one instruction that can come back on a hook acknowledgement: stay alive.
// A relay that exited here would have let the CLI's own dialog through.
func TestHooks_AnAnswerablePromptTellsTheRelayToWait(t *testing.T) {
	uc := &fakeAgentUsecase{
		pendingAwait:  true,
		pendingAnswer: agentusecase.PendingAnswer{ChoiceID: "c1", Wait: 270 * time.Second},
	}
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks",
		[]byte(`{"delivery_id":"d1","segment_id":"seg","provider":"claude",`+
			`"event":"permission","payload_raw":"{}"}`))

	newChatHandlers(uc).Hooks(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	var envelope struct {
		Data struct {
			Await struct {
				ChoiceID string `json:"choiceId"`
				WaitMS   int64  `json:"waitMs"`
			} `json:"await"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Equal(t, "c1", envelope.Data.Await.ChoiceID)
	assert.Equal(t, int64(270_000), envelope.Data.Await.WaitMS)
}

// Every other hook keeps the bare 202 it always had, so a relay with nothing to
// wait for exits immediately.
func TestRegression_HooksKeepsTheBare202WhenNothingIsAnswerable(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks",
		[]byte(`{"delivery_id":"d1","segment_id":"seg","provider":"codex",`+
			`"event":"permission","payload_raw":"{}"}`))

	newChatHandlers(uc).Hooks(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestAwaitHookAnswer_ServesTheRenderedVerdict(t *testing.T) {
	uc := &fakeAgentUsecase{awaitAnswer: agentusecase.HookAnswer{
		Stdout: []byte(`{"hookSpecificOutput":{"hookEventName":"PermissionRequest"}}`),
	}}
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks/await",
		[]byte(`{"delivery_id":"d1"}`))

	newChatHandlers(uc).AwaitHookAnswer(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"d1"}, uc.awaitCalls)
	assert.Contains(t, rec.Body.String(), "hookSpecificOutput")
}

// "Nobody decided" is an outcome, not a failure: the relay prints nothing, exits
// 0, and the CLI's own dialog stands.
func TestAwaitHookAnswer_NoDecisionIsStillA200(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks/await",
		[]byte(`{"delivery_id":"d1"}`))

	newChatHandlers(uc).AwaitHookAnswer(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"stdout":""`)
}

func TestAwaitHookAnswer_RejectsARequestWithNoDelivery(t *testing.T) {
	for _, body := range []string{"{", `{}`, `{"delivery_id":""}`} {
		uc := &fakeAgentUsecase{}
		ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks/await", []byte(body))

		newChatHandlers(uc).AwaitHookAnswer(ctx)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "body %q", body)
		assert.Empty(t, uc.awaitCalls)
	}
}

func TestAbandonHookAnswer_ReportsAndAcknowledges(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks/abandon",
		[]byte(`{"delivery_id":"d1"}`))

	newChatHandlers(uc).AbandonHookAnswer(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, []string{"d1"}, uc.abandonCalls)
}

func TestAbandonHookAnswer_RejectsARequestWithNoDelivery(t *testing.T) {
	for _, body := range []string{"{", `{"delivery_id":""}`} {
		uc := &fakeAgentUsecase{}
		ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks/abandon", []byte(body))

		newChatHandlers(uc).AbandonHookAnswer(ctx)

		assert.Equal(t, http.StatusBadRequest, rec.Code, "body %q", body)
		assert.Empty(t, uc.abandonCalls)
	}
}

func TestAbandonHookAnswer_SurfacesAFailure(t *testing.T) {
	uc := &fakeAgentUsecase{abandonErr: apperr.ErrUnavailable}
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks/abandon",
		[]byte(`{"delivery_id":"d1"}`))

	newChatHandlers(uc).AbandonHookAnswer(ctx)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestAwaitHookAnswer_SurfacesAFailure(t *testing.T) {
	uc := &fakeAgentUsecase{awaitErr: apperr.ErrUnavailable}
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/x/agent/hooks/await",
		[]byte(`{"delivery_id":"d1"}`))

	newChatHandlers(uc).AwaitHookAnswer(ctx)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// A pending prompt whose relay has timed out is still worth drawing — the CLI is
// asking it — but a button on it would reach nobody. A surface that cannot tell
// the two apart offers controls that silently fail.
func TestChoices_StampWhetherAPromptCanStillBeAnswered(t *testing.T) {
	uc := &fakeAgentUsecase{
		pending: []domain.ActivityChoice{
			{ID: "c1", Kind: domain.ChoiceKindPermission, At: activityAt},
			{ID: "c2", Kind: domain.ChoiceKindPermission, At: activityAt},
		},
		answerable: []string{"c1"},
	}
	ctx, rec := scoped(t, "/choices")

	newChatHandlers(inWorkspace(uc)).Choices(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data []struct {
			ID         string `json:"id"`
			Pending    bool   `json:"pending"`
			Answerable bool   `json:"answerable"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data, 2)
	assert.True(t, envelope.Data[0].Pending)
	assert.True(t, envelope.Data[0].Answerable)
	assert.True(t, envelope.Data[1].Pending)
	assert.False(t, envelope.Data[1].Answerable)
}
