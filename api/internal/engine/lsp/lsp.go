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

type engine struct {
	reg    registry.Registry
	mgr    manager.Manager
	mu     sync.Mutex
	snap   map[string][]domlsp.Diagnostic
	userCB func(domlsp.DiagnosticsEvent)
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
		reg:  reg,
		mgr:  mgr,
		snap: make(map[string][]domlsp.Diagnostic),
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
	raw, err := srv.Request(ctx, method, params)
	if err != nil {
		return nil, false, fmt.Errorf("lsp: %s: %w", method, err)
	}
	return raw, true, nil
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
	return e.notify(ctx, wsID, worktreePath, filePath, "textDocument/didChange", params)
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
	return e.notify(ctx, wsID, worktreePath, filePath, "textDocument/didClose", params)
}

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
	if err := srv.Notify(ctx, method, params); err != nil {
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
	spec, ok := e.reg.ForFile(filePath)
	if !ok {
		return
	}
	e.mgr.Release(ctx, wsID, spec.LanguageID)
}
