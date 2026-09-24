package lsp

import (
	"context"
	"encoding/json"
	"fmt"

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
