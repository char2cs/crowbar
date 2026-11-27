package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

type configurableEngine struct {
	pollState engineprovider.ProviderState
	pollErr   error
	branches  []string
	branchErr error
}

func (c configurableEngine) PollOnView(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (engineprovider.ProviderState, error) {
	return c.pollState, c.pollErr
}

func (c configurableEngine) ProtectedBranches(
	_ context.Context,
	_ string,
) ([]string, error) {
	return c.branches, c.branchErr
}

type configurableReader struct {
	all     []domain.Workspace
	listErr error
}

func (c configurableReader) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.all, nil
}

func TestState_NoResolvedWorkspace_500(
	t *testing.T,
) {
	r := newRouterNoWorkspace(configurableEngine{}, configurableReader{})

	rec := do(r, "/v0/chats/chat1/provider")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestState_PollError(
	t *testing.T,
) {
	eng := configurableEngine{pollErr: errors.New("auth failed")}
	r := newRouter(eng, configurableReader{})

	rec := do(r, "/v0/chats/chat1/provider")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestState_Success(
	t *testing.T,
) {
	eng := configurableEngine{pollState: engineprovider.ProviderState{Protected: true}}
	r := newRouter(eng, configurableReader{})

	rec := do(r, "/v0/chats/chat1/provider")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestProtectedBranches_RepoNotFound(
	t *testing.T,
) {
	reader := configurableReader{all: []domain.Workspace{
		{ID: "ws1", RepoID: "other-repo", WorktreePath: "/repo", Provisioning: domain.WorkspaceProvisioned},
	}}
	r := newRouter(configurableEngine{}, reader)

	rec := do(r, "/v0/repos/missing-repo/protected-branches")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProtectedBranches_ListError(
	t *testing.T,
) {
	reader := configurableReader{listErr: errors.New("db down")}
	r := newRouter(configurableEngine{}, reader)

	rec := do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProtectedBranches_EngineError(
	t *testing.T,
) {
	eng := configurableEngine{branchErr: errors.New("provider unreachable")}
	reader := configurableReader{all: []domain.Workspace{
		{ID: "ws1", RepoID: "r1", WorktreePath: "/repo", Provisioning: domain.WorkspaceProvisioned},
	}}
	r := newRouter(eng, reader)

	rec := do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestProtectedBranches_NoBranches(
	t *testing.T,
) {
	eng := configurableEngine{branches: []string{}}
	reader := configurableReader{all: []domain.Workspace{
		{ID: "ws1", RepoID: "r1", WorktreePath: "/repo", Provisioning: domain.WorkspaceProvisioned},
	}}
	r := newRouter(eng, reader)

	rec := do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestProtectedBranches_SomeBranches(
	t *testing.T,
) {
	eng := configurableEngine{branches: []string{"main", "release"}}
	reader := configurableReader{all: []domain.Workspace{
		{ID: "ws0", RepoID: "other", WorktreePath: "/other", Provisioning: domain.WorkspaceProvisioned},
		{ID: "ws1", RepoID: "r1", WorktreePath: "/repo", Provisioning: domain.WorkspaceProvisioned},
	}}
	r := newRouter(eng, reader)

	rec := do(r, "/v0/repos/r1/protected-branches")
	assert.Equal(t, http.StatusOK, rec.Code)
}
