package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	projecthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/projects/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestNew_NilBroadcastDegradesToNoop pins that New tolerates a nil broadcast
// func by substituting a no-op, so a background import that would otherwise
// broadcast the created project never panics.
func TestNew_NilBroadcastDegradesToNoop(
	t *testing.T,
) {
	importer := &fakeImporter{project: domain.Project{ID: "new-id", Name: "gamma"}}
	h := projecthandlers.New(&fakeReader{}, importer, &fakeDeleter{}, nil, unusedHome).WithStat(statOK)

	r := gin.New()
	r.POST("/v0/projects", h.Import)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v0/projects", strings.NewReader(`{"name":"gamma","path":"/g"}`))
	assert.NotPanics(t, func() {
		r.ServeHTTP(rec, req)
	})
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

// A handler built without a crowbar home would resolve one of its own later,
// so New refuses it up front.
func TestNew_NilCrowbarHomePanics(
	t *testing.T,
) {
	assert.Panics(t, func() {
		projecthandlers.New(&fakeReader{}, &fakeImporter{}, &fakeDeleter{}, nil, nil)
	})
}
