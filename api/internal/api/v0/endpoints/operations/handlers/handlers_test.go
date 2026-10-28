package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/operations/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type envelope struct {
	Success bool             `json:"success"`
	Code    string           `json:"code"`
	Data    domain.Operation `json:"data"`
}

func newService(t *testing.T) (*operation.Service, chan domain.Operation) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	opStore, err := storesqlite.NewFromDB[domain.Operation, string](db)
	require.NoError(t, err)
	frames := make(chan domain.Operation, 64)
	return operation.New(opStore, func(op domain.Operation) { frames <- op }, time.Now), frames
}

func dismiss(t *testing.T, svc *operation.Service, id string) (int, envelope) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/v0/operations/:id", handlers.New(svc).Dismiss)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/v0/operations/"+id, nil))
	var env envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return rec.Code, env
}

func spec() operation.Spec {
	return operation.Spec{
		Kind: domain.OpGitPush, Label: "Push main",
		Subject: domain.EntityRef{Kind: domain.EntityWorkspace, ID: "ws1"},
	}
}

func TestDismiss_ReturnsTheDismissedOperation(t *testing.T) {
	svc, _ := newService(t)
	op, err := svc.Run(context.Background(), spec(), func(context.Context, operation.Progress) error { return nil })
	require.NoError(t, err)

	status, env := dismiss(t, svc, op.ID)

	assert.Equal(t, http.StatusOK, status)
	assert.True(t, env.Success)
	assert.Equal(t, op.ID, env.Data.ID)
	assert.True(t, env.Data.Dismissed)
}

func TestDismiss_RunningWorkIs409(t *testing.T) {
	svc, frames := newService(t)
	release := make(chan struct{})
	op, err := svc.Start(context.Background(), spec(), func(context.Context, operation.Progress) error {
		<-release
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		close(release)
		for f := range frames {
			if f.ID == op.ID && f.State == domain.OpSucceeded {
				return
			}
		}
	})

	status, env := dismiss(t, svc, op.ID)

	assert.Equal(t, http.StatusConflict, status)
	assert.False(t, env.Success)
	assert.Equal(t, "operation_running", env.Code)

	status, env = dismiss(t, svc, uuid.NewString())
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, "not_found", env.Code)
}
