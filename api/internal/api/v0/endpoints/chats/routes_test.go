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

type stubUsecase struct{}

func (stubUsecase) CreateChat(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (stubUsecase) ForkChat(
	_ context.Context,
	_ string,
	_ time.Time,
) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (stubUsecase) RenameChat(
	_ context.Context,
	_ string,
	_ string,
) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (stubUsecase) DeleteChat(
	_ context.Context,
	_ string,
	_ time.Time,
) error {
	return nil
}

type stubRepo struct{}

func (stubRepo) ListByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.Chat, error) {
	return nil, nil
}

func TestRegisterMountsRoutes(
	t *testing.T,
) {
	r := gin.New()
	chats.Register(
		r.Group("/v0"),
		stubUsecase{},
		stubRepo{},
		func(_ *gin.Context) {},
		func(_ *gin.Context) {},
	)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v0/workspaces/ws1/chats"},
		{http.MethodGet, "/v0/workspaces/ws1/chats"},
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
