package chats_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chats"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubLifecycle struct{}

func (stubLifecycle) ListChatsByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.Chat, error) {
	return nil, nil
}

func (stubLifecycle) CreateChat(
	_ context.Context,
	_ string,
	_ string,
	_ time.Time,
) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (stubLifecycle) ForkChat(
	_ context.Context,
	_ string,
	_ time.Time,
) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (stubLifecycle) RenameChat(
	_ context.Context,
	_ string,
	_ string,
) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (stubLifecycle) DeleteChat(
	_ context.Context,
	_ string,
	_ time.Time,
) error {
	return nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	chats.Register(r.Group("/v0"), stubLifecycle{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v0/workspaces/w1/chats"},
		{http.MethodPost, "/v0/workspaces/w1/chats"},
		{http.MethodPost, "/v0/chats/c1/fork"},
		{http.MethodPatch, "/v0/chats/c1"},
		{http.MethodDelete, "/v0/chats/c1"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
