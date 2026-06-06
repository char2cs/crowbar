// Package lsp is the public LSP host facade Crowbar's API layer consumes. It
// proxies synchronous editor features (completion, hover, definition,
// references, rename, code action, document symbol) to per-workspace language
// servers, forwards frontend-driven document-sync notifications, and surfaces
// the latest diagnostics snapshot per workspace.
//
// Task-1 decision (10 §1): Crowbar self-implements the minimal JSON-RPC + LSP
// framing surface it needs (id-correlated request/response and
// publishDiagnostics notifications) in internal/protocol, rather than vendoring
// a heavy LSP/JSON-RPC dependency. The protocol surface is small and a third-
// party dependency would be unjustified.
//
// Graceful absence (10 §5): when no language server is configured or installed
// for a file's language, feature methods return an empty result and a nil
// error — never a hard failure — and DiagnosticsSnapshot returns empty until
// diagnostics arrive. This is the documented "LSP server not running" state.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/manager"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

// Engine is the public LSP host surface. Feature methods are synchronous LSP
// requests; the DidOpen/DidChange/DidClose notifications drive document sync;
// diagnostics flow asynchronously through OnDiagnostics and the per-workspace
// snapshot. All methods are safe for concurrent use.
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
	// CodeAction returns the raw textDocument/codeAction result, passed through
	// unchanged.
	CodeAction(
		ctx context.Context,
		wsID string,
		worktreePath string,
		filePath string,
		rng domlsp.Range,
	) (json.RawMessage, error)
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
}

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
}

func spawnProcess(
	_ context.Context,
	spec registry.ServerSpec,
	worktreePath string,
) (server.Server, error) {
	srv, err := server.New(spec.Command, spec.Args, worktreePath)
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
	params := convert.TextDocumentPositionParams(filePath, pos)
	return e.rawRequest(ctx, wsID, worktreePath, filePath, "textDocument/completion", params)
}

func (e *engine) Hover(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
) (json.RawMessage, error) {
	params := convert.TextDocumentPositionParams(filePath, pos)
	return e.rawRequest(ctx, wsID, worktreePath, filePath, "textDocument/hover", params)
}

func (e *engine) Definition(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
) ([]domlsp.Location, error) {
	params := convert.TextDocumentPositionParams(filePath, pos)
	raw, ok, err := e.request(ctx, wsID, worktreePath, filePath, "textDocument/definition", params)
	if err != nil || !ok {
		return nil, err
	}
	return convert.LocationsFromResult(raw)
}

func (e *engine) References(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
) ([]domlsp.Location, error) {
	params := convert.ReferenceParams(filePath, pos)
	raw, ok, err := e.request(ctx, wsID, worktreePath, filePath, "textDocument/references", params)
	if err != nil || !ok {
		return nil, err
	}
	return convert.LocationsFromResult(raw)
}

func (e *engine) Rename(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	pos domlsp.Position,
	newName string,
) (domlsp.WorkspaceEdit, error) {
	params := convert.RenameParams(filePath, pos, newName)
	raw, ok, err := e.request(ctx, wsID, worktreePath, filePath, "textDocument/rename", params)
	if err != nil || !ok {
		return domlsp.WorkspaceEdit{}, err
	}
	return convert.WorkspaceEditFromResult(raw)
}

func (e *engine) CodeAction(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	rng domlsp.Range,
) (json.RawMessage, error) {
	params := convert.CodeActionParams(filePath, rng)
	return e.rawRequest(ctx, wsID, worktreePath, filePath, "textDocument/codeAction", params)
}

func (e *engine) DocumentSymbol(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
) (json.RawMessage, error) {
	params := convert.DocumentSymbolParams(filePath)
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

// request issues a synchronous feature request. ServerForFile acquires a ref
// (spawning if needed) that keeps the server warm for the duration; the
// deferred releaseFile drops it. The acquire/release pair is net-zero, so a
// feature request never permanently inflates the refcount: with an open
// document the matching didOpen ref keeps the server warm across requests; with
// no open document the request spawns, serves, then tears the server down.
func (e *engine) request(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	method string,
	params any,
) (json.RawMessage, bool, error) {
	srv, err := e.mgr.ServerForFile(ctx, wsID, worktreePath, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("lsp: %s: %w", method, err)
	}
	defer e.releaseFile(ctx, wsID, filePath)

	reqCtx, cancel := context.WithTimeout(ctx, e.reqTimeout)
	defer cancel()

	raw, err := srv.Request(reqCtx, method, params)
	if err != nil {
		return nil, false, fmt.Errorf("lsp: %s: %w", method, err)
	}
	return raw, true, nil
}

// releaseFile drops one refcount for the file's language server, resolving the
// languageID from the registry. It mirrors the public Release accounting and is
// the release half of the net-zero acquire/release pairs in request, DidChange,
// and DidClose.
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

func (e *engine) DidOpen(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	languageID string,
	text string,
) error {
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        convert.URIFromPath(filePath),
			"languageId": languageID,
			"version":    1,
			"text":       text,
		},
	}
	// ServerForFile acquires a ref that is HELD for the lifetime of the open
	// document; it is released only in the matching DidClose. This is what keeps
	// the server warm across the feature requests issued while the document is
	// open. On the graceful-absence / spawn-error path notify acquires nothing,
	// so there is no ref to release later.
	return e.notify(ctx, wsID, worktreePath, filePath, "textDocument/didOpen", params)
}

func (e *engine) DidChange(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	text string,
) error {
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":     convert.URIFromPath(filePath),
			"version": 2,
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
	params := map[string]any{
		"textDocument": map[string]any{"uri": convert.URIFromPath(filePath)},
	}
	// DidClose forwards on the ref the matching DidOpen already holds, then drops
	// that ref. RunningServerForFile does NOT acquire, so the single releaseFile
	// below returns the refcount to its pre-open baseline (open→…→close is
	// net-zero). When this drops the last ref the server is torn down. A close
	// with no live server (graceful absence, or close without a prior open) is a
	// no-op: ErrNoServer yields a nil notify and releaseFile is a no-op on an
	// absent entry.
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
