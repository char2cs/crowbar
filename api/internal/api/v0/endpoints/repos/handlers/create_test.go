package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
)

func newCreateRouter(
	store repohandlers.Store,
) *gin.Engine {
	r := gin.New()
	h := repohandlers.New(store)
	rg := r.Group("/v0")
	rg.POST("/repos", h.Create)
	return r
}

func doPost(
	r *gin.Engine,
	target string,
	body any,
) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestCreateSuccess(
	t *testing.T,
) {
	rec := doPost(newCreateRouter(&fakeStore{}), "/v0/repos",
		map[string]any{"id": "r1", "name": "alpha", "path": "/tmp/repo"})
	assert.Equal(t, http.StatusCreated, rec.Code)
}
