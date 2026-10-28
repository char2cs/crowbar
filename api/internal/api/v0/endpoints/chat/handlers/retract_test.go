package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func (*fakeAgentUsecase) Retract(context.Context, string, string) (agentusecase.RetractResult, error) {
	return agentusecase.RetractResult{}, errors.New("fakeAgentUsecase: Retract is retractingUsecase's")
}

func (configurableListGetUsecase) Retract(context.Context, string, string) (agentusecase.RetractResult, error) {
	return agentusecase.RetractResult{}, errors.New("configurableListGetUsecase: Retract is not under test")
}

// retractingUsecase answers Retract with result or err, recording each call.
type retractingUsecase struct {
	*fakeAgentUsecase
	result agentusecase.RetractResult
	err    error
	calls  [][2]string
}

func (r *retractingUsecase) Retract(_ context.Context, chatID, requestID string) (agentusecase.RetractResult, error) {
	r.calls = append(r.calls, [2]string{chatID, requestID})
	return r.result, r.err
}

func retractCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/chat-1/prompts/r1/retract", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}, {Key: "requestId", Value: "r1"}}
	return ctx, rec
}

func TestRetract_RespondsWithStateAndText(t *testing.T) {
	uc := &retractingUsecase{
		fakeAgentUsecase: inWorkspace(&fakeAgentUsecase{}),
		result:           agentusecase.RetractResult{State: "retracted", Text: "rename it"},
	}
	ctx, rec := retractCtx(t)

	newChatHandlers(uc).Retract(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, [][2]string{{"chat-1", "r1"}}, uc.calls)
	var body struct {
		Data dto.AgentRetractDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, dto.AgentRetractDTO{State: "retracted", Text: "rename it"}, body.Data)
}

func TestRetract_ConflictIs409AndUnknownIs404(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{fmt.Errorf("agent: retract: prompt r1 is settled: %w", problem.ErrConflict), http.StatusConflict},
		{fmt.Errorf("agent: retract: prompt r1: %w", problem.ErrNotFound), http.StatusNotFound},
	} {
		uc := &retractingUsecase{fakeAgentUsecase: inWorkspace(&fakeAgentUsecase{}), err: tc.err}
		ctx, rec := retractCtx(t)

		newChatHandlers(uc).Retract(ctx)

		assert.Equal(t, tc.want, rec.Code, rec.Body.String())
	}
}

func TestRetract_RefusesAChatOutsideTheRouteScope(t *testing.T) {
	uc := &retractingUsecase{fakeAgentUsecase: &fakeAgentUsecase{}}
	uc.getChat.ID, uc.getChat.WorkspaceID = "chat-1", "another-ws"
	ctx, rec := retractCtx(t)

	newChatHandlers(uc).Retract(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.calls, "a chat outside the route's scope is never retracted")
}
