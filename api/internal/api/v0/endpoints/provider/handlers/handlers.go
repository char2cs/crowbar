// Package handlers holds the gin handlers backing the read-only provider
// endpoint: the per-workspace provider state (protected flag plus current pull
// request) and the per-repo protected-branch list (02 §2.6). All provider I/O
// is read-only; the engine resolves protected branches and pull-request state
// for open workspaces.
//
// The provider engine is graceful-absent: when its CLI is unavailable the
// engine falls back to default protected branches and an empty state rather
// than failing. A nil engine is guarded before any work runs and yields a 503.
package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

// ProviderEngine is the read-only provider surface the handlers consume: an
// immediate per-workspace poll and the per-repo protected-branch list. It
// mirrors the live provider.Engine methods so the engine satisfies it directly.
type ProviderEngine interface {
	PollOnView(
		ctx context.Context,
		wsID string,
		repoPath string,
		branch string,
	) (providertypes.ProviderState, error)
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)
}

// WorkspaceReader resolves a workspace id to its aggregate, supplying the
// worktree path the provider engine operates against. It mirrors the workspace
// repository's Get method.
type WorkspaceReader interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// RepoReader resolves a repo id to its persisted row, supplying the repo root
// path the provider engine resolves protected branches against. It mirrors the
// generic repository store's FindByKey method, returning a nil row (and nil
// error) when the id is unknown.
type RepoReader interface {
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Repository, error)
}

// Handlers serves the /v0 provider routes from the provider engine, the
// workspace reader, and the repo reader. A nil provider engine surfaces as a 503
// on every route.
type Handlers struct {
	provider ProviderEngine
	wsReader WorkspaceReader
	repos    RepoReader
}

// New builds the provider Handlers from the provider engine, the workspace
// reader, and the repo reader.
func New(
	provider ProviderEngine,
	wsReader WorkspaceReader,
	repos RepoReader,
) *Handlers {
	return &Handlers{
		provider: provider,
		wsReader: wsReader,
		repos:    repos,
	}
}

func (h *Handlers) requireProvider(
	c *gin.Context,
) bool {
	if h.provider == nil {
		libs.WriteErr(
			c,
			http.StatusServiceUnavailable,
			"provider engine not available",
		)
		return false
	}
	return true
}

func (h *Handlers) workspace(
	c *gin.Context,
	idParam string,
) (domain.Workspace, bool) {
	row, err := h.wsReader.Get(
		c.Request.Context(),
		c.Param(idParam),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return domain.Workspace{}, false
	}
	return row, true
}
