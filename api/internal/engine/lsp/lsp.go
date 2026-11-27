package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/manager"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

type Engine interface {
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
	DidSave(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) error
	DidClose(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) error
	OnDiagnostics(
		fn func(domlsp.DiagnosticsEvent),
	)
	DiagnosticsSnapshot(
		wsID string,
	) []domlsp.Diagnostic
	Release(
		ctx context.Context,
		wsID string,
		filePath string,
	)
	ReleaseWorkspace(
		ctx context.Context,
		wsID string,
	)
	Shutdown(
		ctx context.Context,
	)
}

const restartTimeout = 30 * time.Second

const DefaultRequestTimeout = 10 * time.Second

type engine struct {
	reg        registry.Registry
	mgr        manager.Manager
	reqTimeout time.Duration
	mu         sync.Mutex
	snap       map[string][]domlsp.Diagnostic
	userCB     func(domlsp.DiagnosticsEvent)
	versions   map[string]int
}

func New(
	overrides map[string]registry.ServerSpec,
) Engine {
	reg := registry.New(overrides)
	return newWithManager(reg, manager.New(reg, spawnProcess))
}

func newWithManager(
	reg registry.Registry,
	mgr manager.Manager,
) *engine {
	e := &engine{
		reg:        reg,
		mgr:        mgr,
		reqTimeout: DefaultRequestTimeout,
		snap:       make(map[string][]domlsp.Diagnostic),
		versions:   make(map[string]int),
	}
	e.mgr.OnDiagnostics(e.onDiagnostics)
	e.mgr.OnReleaseEmpty(e.evictSnapshot)
	return e
}

func spawnProcess(
	ctx context.Context,
	spec registry.ServerSpec,
	worktreePath string,
) (server.Server, error) {
	srv, err := server.New(ctx, spec.Command, spec.Args, worktreePath, spec.InitializationOptions)
	if err != nil {
		return nil, fmt.Errorf("lsp: spawn %s: %w", spec.Command, err)
	}
	return srv, nil
}

func (e *engine) Status(
	wsID string,
	filePath string,
) domlsp.ServerStatus {
	return e.mgr.Status(wsID, filePath)
}

func (e *engine) Restart(
	ctx context.Context,
	wsID string,
	filePath string,
) (domlsp.ServerStatus, error) {
	srv, err := e.mgr.RunningServerForFile(wsID, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return e.mgr.Status(wsID, filePath), nil
	}
	if err != nil {
		return e.mgr.Status(wsID, filePath), fmt.Errorf("lsp: restart: %w", err)
	}
	restartCtx, cancel := context.WithTimeout(ctx, restartTimeout)
	defer cancel()
	if err := srv.Replay(restartCtx); err != nil {
		return e.mgr.Status(wsID, filePath), fmt.Errorf("lsp: restart: %w", err)
	}
	return e.mgr.Status(wsID, filePath), nil
}

func (e *engine) releaseFile(
	ctx context.Context,
	wsID string,
	filePath string,
) {
	spec, ok := e.reg.ForFile(filePath)
	if !ok {
		return
	}
	e.mgr.Release(ctx, wsID, spec.LanguageID)
}

func (e *engine) Release(
	ctx context.Context,
	wsID string,
	filePath string,
) {
	e.releaseFile(ctx, wsID, filePath)
}

func (e *engine) ReleaseWorkspace(
	ctx context.Context,
	wsID string,
) {
	e.mgr.ReleaseWorkspace(ctx, wsID)
}

func (e *engine) Shutdown(
	ctx context.Context,
) {
	e.mgr.Shutdown(ctx)
}
