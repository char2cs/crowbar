// Package lsp proxies editor features and document sync to per-workspace language servers over a
// minimal self-implemented JSON-RPC (a heavy dependency is unjustified). With no server installed,
// features return empty results and a nil error: the "not running" state, never a failure.
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

// Engine is the public LSP host surface: synchronous feature requests, DidOpen/Change/Close
// sync, and asynchronous diagnostics. Safe for concurrent use.
type Engine interface {
	// Completion returns the raw textDocument/completion result. The frontend
	// speaks LSP/Monaco, so the wire payload is passed through unchanged. An
	// absent server yields a nil result and nil error.
	Completion(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) (json.RawMessage, error)
	// Hover returns the raw textDocument/hover result, passed through unchanged.
	Hover(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) (json.RawMessage, error)
	// Definition returns the textDocument/definition result as Crowbar Locations.
	Definition(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) ([]domlsp.Location, error)
	// References returns the textDocument/references result as Crowbar Locations.
	References(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) ([]domlsp.Location, error)
	// Rename returns the textDocument/rename result as a Crowbar WorkspaceEdit.
	Rename(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
		newName string,
	) (domlsp.WorkspaceEdit, error)
	// CodeAction returns the raw codeAction result, forwarding diagnostics (JSON or nil) as
	// context for quick fixes; workspace edits come back workspace-relative.
	CodeAction(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		rng domlsp.Range,
		diagnostics json.RawMessage,
	) (json.RawMessage, error)
	// SignatureHelp returns the raw textDocument/signatureHelp result.
	SignatureHelp(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		pos domlsp.Position,
	) (json.RawMessage, error)
	// CodeLens returns the raw textDocument/codeLens result.
	CodeLens(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) (json.RawMessage, error)
	// CodeLensResolve returns the raw codeLens/resolve result for one lens
	// previously returned by CodeLens.
	CodeLensResolve(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		lens json.RawMessage,
	) (json.RawMessage, error)
	// SemanticTokens returns the file's tokens in the semtok legend, as a delta when supported
	// and previousResultID is set; nil when no server offers full tokens.
	SemanticTokens(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		previousResultID string,
	) (json.RawMessage, error)
	// SemanticTokensRange returns the semantic tokens of rng in the canonical
	// legend, or nil when no server offers range semantic tokens.
	SemanticTokensRange(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		rng domlsp.Range,
	) (json.RawMessage, error)
	// ExecuteCommand runs a command a server issued (on a code lens or code
	// action) and returns its result plus the workspace edits the server
	// applied through the editor while it ran, workspace-relative.
	ExecuteCommand(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		command string,
		arguments json.RawMessage,
	) (domlsp.CommandResult, error)
	// Formatting returns the raw textDocument/formatting result (TextEdit[]).
	Formatting(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		options domlsp.FormattingOptions,
	) (json.RawMessage, error)
	// Status reports which server serves filePath's language in wsID and
	// whether it is running. It never spawns.
	Status(
		wsID string,
		filePath string,
	) domlsp.ServerStatus
	// Restart respawns filePath's running server and reopens its documents; a stopped one is
	// left to start on the next open. The resulting status is returned.
	Restart(
		ctx context.Context,
		wsID string,
		filePath string,
	) (domlsp.ServerStatus, error)
	// DocumentSymbol returns the raw textDocument/documentSymbol result, passed
	// through unchanged.
	DocumentSymbol(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) (json.RawMessage, error)
	// DidOpen forwards a textDocument/didOpen notification and records the URI in
	// the server's open-document set.
	DidOpen(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		languageID string,
		text string,
	) error
	// DidChange forwards a textDocument/didChange notification with the full
	// buffer text.
	DidChange(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		text string,
	) error
	// DidSave forwards a textDocument/didSave notification to the running
	// server (servers such as rust-analyzer and gopls re-check on save). It
	// never spawns and never changes the refcount.
	DidSave(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) error
	// DidClose forwards a textDocument/didClose notification and drops the URI
	// from the server's open-document set.
	DidClose(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
	) error
	// OnDiagnostics registers the callback invoked for every diagnostics event,
	// in addition to updating the per-workspace snapshot.
	OnDiagnostics(
		fn func(domlsp.DiagnosticsEvent),
	)
	// DiagnosticsSnapshot returns the latest diagnostics recorded for wsID, or an
	// empty slice if none have arrived.
	DiagnosticsSnapshot(
		wsID string,
	) []domlsp.Diagnostic
	// Release decrements the LSP refcount for the file's language server, closing
	// it once the last subscriber releases.
	Release(
		ctx context.Context,
		wsID string,
		filePath string,
	)
	// ReleaseWorkspace tears down every server for wsID regardless of refcount, so a crash or
	// drop with documents open (DidClose never ran) never leaks server processes.
	ReleaseWorkspace(
		ctx context.Context,
		wsID string,
	)
	// Shutdown closes every language server in the pool regardless of refcount.
	// engine.Container.Close calls it on daemon shutdown so no spawned gopls /
	// tsserver / rust-analyzer survives to leak RAM, CPU, and pipe FDs.
	Shutdown(
		ctx context.Context,
	)
}

// restartTimeout bounds a restart: respawn plus the initialize handshake,
// which is slow on a cold language server.
const restartTimeout = 30 * time.Second

// DefaultRequestTimeout bounds each synchronous LSP feature request and each
// document-sync notification. A wedged language server degrades to a timeout
// error instead of hanging the handler goroutine forever.
const DefaultRequestTimeout = 10 * time.Second

type engine struct {
	reg        registry.Registry
	mgr        manager.Manager
	reqTimeout time.Duration
	mu         sync.Mutex
	snap       map[string][]domlsp.Diagnostic
	userCB     func(domlsp.DiagnosticsEvent)
	// versions is the per-open-document version counter, keyed by
	// docKey(wsID, uri). LSP requires every didChange to carry a version
	// greater than the last one.
	versions map[string]int
}

// New returns an Engine backed by a registry seeded with overrides and a
// manager that spawns real language-server processes. Pass nil overrides to use
// only the built-in defaults.
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

// releaseFile drops one refcount for the file's server: the release half of the net-zero
// pairs in request, DidChange and DidClose.
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
