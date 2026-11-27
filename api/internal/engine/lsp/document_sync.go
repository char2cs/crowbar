package lsp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/manager"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

func docKey(
	wsID string,
	uri string,
) string {
	return wsID + "|" + uri
}

func (e *engine) openVersion(
	key string,
) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.versions[key] = 1
	return 1
}

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

func (e *engine) DidOpen(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	languageID string,
	text string,
) error {
	abs := absFilePath(worktreePath, filePath)
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
	return e.notify(ctx, wsID, worktreePath, filePath, "textDocument/didOpen", params, true)
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
	return e.notify(ctx, wsID, worktreePath, filePath, "textDocument/didChange", params, false)
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
	return e.notifyRunning(ctx, wsID, filePath, "textDocument/didClose", params, true)
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
	return e.notifyRunning(ctx, wsID, filePath, "textDocument/didSave", params, false)
}

func (e *engine) notify(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	method string,
	params any,
	hold bool,
) error {
	srv, err := e.mgr.ServerForFile(ctx, wsID, worktreePath, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lsp: %s: %w", method, err)
	}
	if !hold {
		defer e.releaseFile(ctx, wsID, filePath)
	}
	return e.send(ctx, srv, method, params)
}

func (e *engine) notifyRunning(
	ctx context.Context,
	wsID string,
	filePath string,
	method string,
	params any,
	release bool,
) error {
	srv, err := e.mgr.RunningServerForFile(wsID, filePath)
	if errors.Is(err, manager.ErrNoServer) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lsp: %s: %w", method, err)
	}
	if release {
		defer e.releaseFile(ctx, wsID, filePath)
	}
	return e.send(ctx, srv, method, params)
}

func (e *engine) send(
	ctx context.Context,
	srv server.Server,
	method string,
	params any,
) error {
	notifyCtx, cancel := context.WithTimeout(ctx, e.reqTimeout)
	defer cancel()
	if err := srv.Notify(notifyCtx, method, params); err != nil {
		return fmt.Errorf("lsp: %s: %w", method, err)
	}
	return nil
}
