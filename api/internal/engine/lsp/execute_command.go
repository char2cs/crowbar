package lsp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

func (e *engine) ExecuteCommand(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	command string,
	arguments json.RawMessage,
) (domlsp.CommandResult, error) {
	out := domlsp.CommandResult{Edits: []json.RawMessage{}}
	params := convert.ExecuteCommandParams(command, arguments)
	_, err := e.serve(ctx, wsID, worktreePath, filePath, func(reqCtx context.Context, srv server.Server) error {
		if !srv.CanExecute(command) {
			return fmt.Errorf("%w: the language server does not run %q", apperr.ErrInvalidArgument, command)
		}
		result, edits, err := srv.ExecuteCommand(reqCtx, params)
		if err != nil {
			return err
		}
		out.Result = result
		for _, edit := range edits {
			out.Edits = append(out.Edits, convert.RelWorkspaceEdit(worktreePath, edit))
		}
		return nil
	})
	if err != nil {
		return domlsp.CommandResult{}, fmt.Errorf("lsp: execute command %s: %w", command, err)
	}
	return out, nil
}

// commandRequest issues a request whose result carries commands (code lenses,
// code actions) and passes it through toClient with the server's own
// command set, so the editor is only offered commands the server can run.
func (e *engine) commandRequest(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	method string,
	params any,
	toClient func(raw json.RawMessage, canRun func(string) bool) json.RawMessage,
) (json.RawMessage, error) {
	var out json.RawMessage
	_, err := e.serve(ctx, wsID, worktreePath, filePath, func(reqCtx context.Context, srv server.Server) error {
		raw, err := srv.Request(reqCtx, method, params)
		if err != nil || len(raw) == 0 {
			return err
		}
		out = toClient(raw, srv.CanExecute)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lsp: %s: %w", method, err)
	}
	return out, nil
}
