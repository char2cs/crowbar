package handlers_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSyncSuccess(
	t *testing.T,
) {
	reader := &fakeReader{synced: domain.Workspace{ID: "ws1"}}
	r := newRouter(reader, &fakeHierarchy{}, &fakeRepos{repo: &domain.Repository{ID: "r1"}})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/sync", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws1", reader.gotSync)
}

func TestSyncError(
	t *testing.T,
) {
	reader := &fakeReader{syncErr: errors.New("db down")}
	r := newRouter(reader, &fakeHierarchy{}, &fakeRepos{})

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/sync", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
