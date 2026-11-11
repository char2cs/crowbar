package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/manager"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

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
