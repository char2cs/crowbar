package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func answerRequest(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, rec := newTestContext(t, http.MethodPost,
		"/v0/projects/p/repos/r/workspaces/ws-1/chats/chat-1/choices/c1/answer",
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

// A held ask nobody decided in time answers with no body: the CLI asks it itself.
func TestHTTPHook_AHeldAskNobodyDecidedAnswersEmpty(t *testing.T) {
	uc := &fakeAgentUsecase{validToken: "tok", hookAsk: "c1", pendingAwait: true}
	ctx, rec := newHTTPHookContext(t, "tok", []byte(`{}`))

	newChatHandlers(uc).HTTPHook(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestHTTPHook_AFailedWaitIsMapped(t *testing.T) {
	uc := &fakeAgentUsecase{validToken: "tok", hookAsk: "c1", pendingAwait: true, awaitErr: apperr.ErrUnavailable}
	ctx, rec := newHTTPHookContext(t, "tok", []byte(`{}`))

	newChatHandlers(uc).HTTPHook(ctx)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

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
