package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

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
		diagnostics json.RawMessage,
	) (json.RawMessage, error)
	SignatureHelp(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) (json.RawMessage, error)
	CodeLens(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) (json.RawMessage, error)
	CodeLensResolve(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		lens json.RawMessage,
	) (json.RawMessage, error)
	SemanticTokens(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		previousResultID string,
	) (json.RawMessage, error)
	SemanticTokensRange(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		rng domlsp.Range,
	) (json.RawMessage, error)
	ExecuteCommand(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		command string,
		arguments json.RawMessage,
	) (domlsp.CommandResult, error)
	Formatting(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		options domlsp.FormattingOptions,
	) (json.RawMessage, error)
	Status(
		wsID string,
		filePath string,
	) domlsp.ServerStatus
	Restart(
		ctx context.Context,
		wsID string,
		filePath string,
	) (domlsp.ServerStatus, error)
	DidSave(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) error
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

type GitEngine interface {
	Blame(
		ctx context.Context,
		repoPath string,
		filePath string,
	) ([]gitdomain.BlameEntry, error)
}

type Handlers struct {
	lsp LSPEngine
	git GitEngine
}

func New(
	lsp LSPEngine,
	git GitEngine,
) *Handlers {
	return &Handlers{
		lsp: lsp,
		git: git,
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

func (h *Handlers) workspace(
	c *gin.Context,
) (domain.Workspace, bool) {
	ws, ok := reqscope.Workspace(c)
	if !ok {
		libs.WriteErr(c, http.StatusInternalServerError, "chat worktree not resolved")
		return domain.Workspace{}, false
	}
	return ws, true
}

func (h *Handlers) worktreePath(
	c *gin.Context,
) (string, bool) {
	ws, ok := h.workspace(c)
	if !ok {
		return "", false
	}
	return ws.WorktreePath, true
}

func (h *Handlers) lspOwnerID(
	c *gin.Context,
) string {
	return c.Param("chatId")
}
