// Package handlers holds the gin handlers backing the editor endpoint: git
// blame, the synchronous LSP feature requests (completion, hover, definition,
// references, rename, code action, document symbol), the document-sync
// notifications (didOpen, didChange, didClose), and the diagnostics snapshot
// (02 §2.5, 04 §3, 10).
//
// The LSP engine is graceful-absent: when no language server is installed for a
// file's language the feature methods return empty results and a nil error
// rather than failing, so those responses still carry a 200 envelope. The
// engine and git engine are guarded for nil before any work runs; an absent
// engine yields a 503.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

// LSPEngine is the LSP host surface the editor handlers consume. It mirrors the
// live enginelsp.Engine feature, document-sync, and diagnostics-snapshot
// methods so the engine satisfies it directly.
type LSPEngine interface {
	Completion(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) (json.RawMessage, error)
	Hover(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) (json.RawMessage, error)
	Definition(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) ([]domlsp.Location, error)
	References(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) ([]domlsp.Location, error)
	Rename(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
		newName string,
	) (domlsp.WorkspaceEdit, error)
	CodeAction(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		rng domlsp.Range,
	) (json.RawMessage, error)
	DocumentSymbol(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) (json.RawMessage, error)
	DidOpen(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		languageID string,
		text string,
	) error
	DidChange(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		text string,
	) error
	DidClose(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) error
	DiagnosticsSnapshot(
		wsID string,
	) []domlsp.Diagnostic
}

// GitEngine is the git surface the editor handlers consume: the per-line blame
// annotation. It mirrors enginegit.Engine so the engine satisfies it directly.
type GitEngine interface {
	Blame(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.BlameEntry, error)
}

// WorkspaceReader resolves a workspace id to its aggregate, supplying the
// worktree path the LSP and git engines operate against. It mirrors the
// workspace repository's Get method.
type WorkspaceReader interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
}

// Handlers serves the /v0 editor routes from the LSP host, the git engine, and
// the workspace reader. A nil lsp or git engine surfaces as a 503 on the
// affected route.
type Handlers struct {
	lsp      LSPEngine
	git      GitEngine
	wsReader WorkspaceReader
}

// New builds the editor Handlers from the LSP host, the git engine, and the
// workspace reader.
func New(
	lsp LSPEngine,
	git GitEngine,
	wsReader WorkspaceReader,
) *Handlers {
	return &Handlers{
		lsp:      lsp,
		git:      git,
		wsReader: wsReader,
	}
}

func (h *Handlers) requireLSP(
	c *gin.Context,
) bool {
	if h.lsp == nil {
		libs.WriteErr(
			c,
			http.StatusServiceUnavailable,
			"lsp engine not available",
		)
		return false
	}
	return true
}

func (h *Handlers) requireGit(
	c *gin.Context,
) bool {
	if h.git == nil {
		libs.WriteErr(
			c,
			http.StatusServiceUnavailable,
			"git engine not available",
		)
		return false
	}
	return true
}

func (h *Handlers) worktreePath(
	c *gin.Context,
) (string, bool) {
	row, err := h.wsReader.Get(
		c.Request.Context(),
		c.Param("wsId"),
	)
	if err != nil {
		libs.WriteErr(
			c,
			http.StatusNotFound,
			"workspace not found",
		)
		return "", false
	}
	return row.WorktreePath, true
}
