// Package lsp proxies editor features and document sync to per-workspace language servers over a
// minimal self-implemented JSON-RPC (a heavy dependency is unjustified). With no server installed,
// features return empty results and a nil error: the "not running" state, never a failure.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
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
// error instead of hanging the handler goroutine forever (10 §5).
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

func (e *engine) evictSnapshot(
	wsID string,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.snap, wsID)
	prefix := wsID + "|"
	for key := range e.versions {
		if strings.HasPrefix(key, prefix) {
			delete(e.versions, key)
		}
	}
}

func docKey(
	wsID string,
	uri string,
) string {
	return wsID + "|" + uri
}

// openVersion resets a document's version on (re)open and returns it.
func (e *engine) openVersion(
	key string,
) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.versions[key] = 1
	return 1
}

// nextVersion returns the version for the next didChange of a document.
func (e *engine) nextVersion(
	key string,
) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.versions[key]++
	if e.versions[key] < 2 {
		e.versions[key] = 2
	}
	return e.versions[key]
}

func (e *engine) forgetVersion(
	key string,
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.versions, key)
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

func (e *engine) Completion(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
) (json.RawMessage, error) {
	params := convert.TextDocumentPositionParams(absFilePath(worktreePath, filePath), pos)
	return e.rawRequest(ctx, wsID, worktreePath, filePath, "textDocument/completion", params)
}

func (e *engine) Hover(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
) (json.RawMessage, error) {
	params := convert.TextDocumentPositionParams(absFilePath(worktreePath, filePath), pos)
	return e.rawRequest(ctx, wsID, worktreePath, filePath, "textDocument/hover", params)
}

func (e *engine) Definition(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
) ([]domlsp.Location, error) {
	params := convert.TextDocumentPositionParams(absFilePath(worktreePath, filePath), pos)
	raw, ok, err := e.request(ctx, wsID, worktreePath, filePath, "textDocument/definition", params)
	if err != nil || !ok {
		return nil, err
	}
	locs, err := convert.LocationsFromResult(raw)
	if err != nil {
		return nil, err
	}
	return relLocations(worktreePath, locs), nil
}

func (e *engine) References(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
) ([]domlsp.Location, error) {
	params := convert.ReferenceParams(absFilePath(worktreePath, filePath), pos)
	raw, ok, err := e.request(ctx, wsID, worktreePath, filePath, "textDocument/references", params)
	if err != nil || !ok {
		return nil, err
	}
	locs, err := convert.LocationsFromResult(raw)
	if err != nil {
		return nil, err
	}
	return relLocations(worktreePath, locs), nil
}

func (e *engine) Rename(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
	newName string,
) (domlsp.WorkspaceEdit, error) {
	params := convert.RenameParams(absFilePath(worktreePath, filePath), pos, newName)
	raw, ok, err := e.request(ctx, wsID, worktreePath, filePath, "textDocument/rename", params)
	if err != nil || !ok {
		return domlsp.WorkspaceEdit{}, err
	}
	edit, err := convert.WorkspaceEditFromResult(raw)
	if err != nil {
		return domlsp.WorkspaceEdit{}, err
	}
	return relWorkspaceEdit(worktreePath, edit), nil
}

func (e *engine) CodeAction(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	rng domlsp.Range,
	diagnostics json.RawMessage,
) (json.RawMessage, error) {
	params := convert.CodeActionParams(absFilePath(worktreePath, filePath), rng, diagnostics)
	return e.commandRequest(ctx, wsID, worktreePath, filePath, "textDocument/codeAction", params,
		func(raw json.RawMessage, canRun func(string) bool) json.RawMessage {
			return convert.ClientCodeActions(worktreePath, raw, canRun)
		})
}

func (e *engine) SignatureHelp(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
) (json.RawMessage, error) {
	params := convert.TextDocumentPositionParams(absFilePath(worktreePath, filePath), pos)
	return e.rawRequest(ctx, wsID, worktreePath, filePath, "textDocument/signatureHelp", params)
}

func (e *engine) CodeLens(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
) (json.RawMessage, error) {
	params := convert.DocumentSymbolParams(absFilePath(worktreePath, filePath))
	return e.commandRequest(ctx, wsID, worktreePath, filePath, "textDocument/codeLens", params,
		convert.ClientLenses)
}

func (e *engine) CodeLensResolve(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	lens json.RawMessage,
) (json.RawMessage, error) {
	return e.commandRequest(ctx, wsID, worktreePath, filePath, "codeLens/resolve", lens,
		convert.ClientLenses)
}

func (e *engine) Formatting(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	options domlsp.FormattingOptions,
) (json.RawMessage, error) {
	params := convert.FormattingParams(absFilePath(worktreePath, filePath), options)
	return e.rawRequest(ctx, wsID, worktreePath, filePath, "textDocument/formatting", params)
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

func (e *engine) DocumentSymbol(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
) (json.RawMessage, error) {
	params := convert.DocumentSymbolParams(absFilePath(worktreePath, filePath))
	return e.rawRequest(ctx, wsID, worktreePath, filePath, "textDocument/documentSymbol", params)
}

func (e *engine) rawRequest(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	method string,
	params any,
) (json.RawMessage, error) {
	raw, ok, err := e.request(ctx, wsID, worktreePath, filePath, method, params)
	if err != nil || !ok {
		return nil, err
	}
	return raw, nil
}

// request acquires a server ref for its duration and releases it: net-zero, so an open
// document keeps the server warm, and with none the request spawns, serves and tears down.
func (e *engine) request(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	method string,
	params any,
) (json.RawMessage, bool, error) {
	var raw json.RawMessage
	ok, err := e.serve(ctx, wsID, worktreePath, filePath, func(reqCtx context.Context, srv server.Server) error {
		var err error
		raw, err = srv.Request(reqCtx, method, params)
		return err
	})
	if err != nil {
		return nil, false, fmt.Errorf("lsp: %s: %w", method, err)
	}
	return raw, ok, nil
}

// serve runs fn against the file's language server with the request timeout,
// under the same net-zero acquire/release as request. It reports false (and
// never calls fn) when no server serves the file.
func (e *engine) serve(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	fn func(reqCtx context.Context, srv server.Server) error,
) (bool, error) {
	srv, err := e.mgr.ServerForFile(ctx, wsID, worktreePath, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer e.releaseFile(ctx, wsID, filePath)

	reqCtx, cancel := context.WithTimeout(ctx, e.reqTimeout)
	defer cancel()
	return true, fn(reqCtx, srv)
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

// absFilePath resolves a workspace-relative file path against the worktree so
// the language server receives an absolute file URI it can match to its rooted
// workspace. An already-absolute path is returned unchanged.
func absFilePath(
	worktreePath string,
	filePath string,
) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	return filepath.Join(worktreePath, filePath)
}

// relLocations rewrites FilePaths to workspace-relative form: the frontend addresses files
// that way, so the server's absolute paths never reach it.
func relLocations(
	worktreePath string,
	locations []domlsp.Location,
) []domlsp.Location {
	for i := range locations {
		locations[i].FilePath = convert.WorkspaceRelPath(worktreePath, locations[i].FilePath)
	}
	return locations
}

// relWorkspaceEdit rewrites a workspace edit's per-file keys to their
// workspace-relative form, so a rename's edits address the same paths the
// editor's buffers and the files API use.
func relWorkspaceEdit(
	worktreePath string,
	edit domlsp.WorkspaceEdit,
) domlsp.WorkspaceEdit {
	if len(edit.Changes) == 0 {
		return edit
	}

	changes := make(map[string][]domlsp.TextEdit, len(edit.Changes))
	for path, edits := range edit.Changes {
		changes[convert.WorkspaceRelPath(worktreePath, path)] = edits
	}
	return domlsp.WorkspaceEdit{Changes: changes}
}

func (e *engine) DidOpen(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	languageID string,
	text string,
) error {
	abs := absFilePath(worktreePath, filePath)
	// A document outside the worktree belongs to another workspace's server
	// (or none); opening it here would spawn and feed the wrong one.
	if rel, err := filepath.Rel(worktreePath, abs); err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("lsp: textDocument/didOpen: %w: %q is outside the workspace",
			problem.ErrInvalidArgument, filePath)
	}
	uri := convert.URIFromPath(abs)
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": languageID,
			"version":    e.openVersion(docKey(wsID, uri)),
			"text":       text,
		},
	}
	// The acquired ref is HELD until the matching DidClose, keeping the server warm while the
	// document is open; the absence/spawn-error path acquires nothing.
	return e.notify(ctx, wsID, worktreePath, filePath, "textDocument/didOpen", params)
}

func (e *engine) DidChange(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	text string,
) error {
	uri := convert.URIFromPath(absFilePath(worktreePath, filePath))
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":     uri,
			"version": e.nextVersion(docKey(wsID, uri)),
		},
		"contentChanges": []any{map[string]any{"text": text}},
	}
	// didChange is net-zero: notifyAndRelease acquires for the forward and
	// releases immediately after, so it never permanently changes the refcount.
	// The server stays warm only because of the ref held by the matching DidOpen.
	return e.notifyAndRelease(ctx, wsID, worktreePath, filePath, "textDocument/didChange", params)
}

func (e *engine) DidClose(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
) error {
	uri := convert.URIFromPath(absFilePath(worktreePath, filePath))
	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
	}
	e.forgetVersion(docKey(wsID, uri))
	// RunningServerForFile does not acquire, so the one releaseFile returns the refcount to its
	// pre-open baseline (tearing down on the last ref); with no live server both are no-ops.
	srv, err := e.mgr.RunningServerForFile(wsID, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lsp: textDocument/didClose: %w", err)
	}
	defer e.releaseFile(ctx, wsID, filePath)

	notifyCtx, cancel := context.WithTimeout(ctx, e.reqTimeout)
	defer cancel()

	if err := srv.Notify(notifyCtx, "textDocument/didClose", params); err != nil {
		return fmt.Errorf("lsp: textDocument/didClose: %w", err)
	}
	return nil
}

func (e *engine) DidSave(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
) error {
	params := map[string]any{
		"textDocument": map[string]any{
			"uri": convert.URIFromPath(absFilePath(worktreePath, filePath)),
		},
	}
	// Like didClose, didSave only makes sense for a document the server has
	// open, so it rides the running server without acquiring a ref.
	srv, err := e.mgr.RunningServerForFile(wsID, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lsp: textDocument/didSave: %w", err)
	}
	notifyCtx, cancel := context.WithTimeout(ctx, e.reqTimeout)
	defer cancel()
	if err := srv.Notify(notifyCtx, "textDocument/didSave", params); err != nil {
		return fmt.Errorf("lsp: textDocument/didSave: %w", err)
	}
	return nil
}

// notify forwards a notification and HOLDS the ref acquired by ServerForFile.
// It is the acquiring half used by DidOpen, whose ref lives until DidClose.
func (e *engine) notify(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	method string,
	params any,
) error {
	srv, err := e.mgr.ServerForFile(ctx, wsID, worktreePath, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lsp: %s: %w", method, err)
	}

	notifyCtx, cancel := context.WithTimeout(ctx, e.reqTimeout)
	defer cancel()

	if err := srv.Notify(notifyCtx, method, params); err != nil {
		return fmt.Errorf("lsp: %s: %w", method, err)
	}
	return nil
}

// notifyAndRelease forwards a notification and RELEASES the ref it acquired,
// making the call net-zero. It is used by DidChange, which must not change the
// refcount: the server stays warm only on the ref held by DidOpen.
func (e *engine) notifyAndRelease(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	method string,
	params any,
) error {
	srv, err := e.mgr.ServerForFile(ctx, wsID, worktreePath, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lsp: %s: %w", method, err)
	}
	defer e.releaseFile(ctx, wsID, filePath)

	notifyCtx, cancel := context.WithTimeout(ctx, e.reqTimeout)
	defer cancel()

	if err := srv.Notify(notifyCtx, method, params); err != nil {
		return fmt.Errorf("lsp: %s: %w", method, err)
	}
	return nil
}

func (e *engine) OnDiagnostics(
	fn func(domlsp.DiagnosticsEvent),
) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.userCB = fn
}

func (e *engine) onDiagnostics(
	event domlsp.DiagnosticsEvent,
) {
	e.mu.Lock()
	e.snap[event.WsID] = event.Diagnostics
	cb := e.userCB
	e.mu.Unlock()
	if cb != nil {
		cb(event)
	}
}

func (e *engine) DiagnosticsSnapshot(
	wsID string,
) []domlsp.Diagnostic {
	e.mu.Lock()
	defer e.mu.Unlock()
	diags, ok := e.snap[wsID]
	if !ok {
		return []domlsp.Diagnostic{}
	}
	return diags
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
