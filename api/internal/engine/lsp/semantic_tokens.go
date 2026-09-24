package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

func (e *engine) SemanticTokens(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	previousResultID string,
) (json.RawMessage, error) {
	path := absFilePath(worktreePath, filePath)
	var out json.RawMessage
	_, err := e.serve(ctx, wsID, worktreePath, filePath, func(reqCtx context.Context, srv server.Server) error {
		support := srv.SemanticTokens()
		if !support.Full() {
			return nil
		}
		var err error
		out, err = semanticTokensDelta(reqCtx, srv, support, path, previousResultID)
		if !errors.Is(err, errFullNeeded) {
			return err
		}
		raw, err := srv.Request(reqCtx, "textDocument/semanticTokens/full",
			convert.DocumentSymbolParams(path))
		if err != nil {
			return err
		}
		out, err = support.Remap(raw)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("lsp: semantic tokens: %w", err)
	}
	return out, nil
}

// errFullNeeded reports that a delta cannot answer: none was asked for, the
// server offers none, or its edits cannot be remapped.
var errFullNeeded = errors.New("lsp: full semantic tokens needed")

// semanticTokensDelta asks for edits against previousResultID, or fails with
// errFullNeeded when only a full result can answer.
func semanticTokensDelta(
	ctx context.Context,
	srv server.Server,
	support semtok.Support,
	path string,
	previousResultID string,
) (json.RawMessage, error) {
	if previousResultID == "" || !support.Delta() {
		return nil, errFullNeeded
	}
	raw, err := srv.Request(ctx, "textDocument/semanticTokens/full/delta",
		convert.SemanticTokensDeltaParams(path, previousResultID))
	if err != nil && ctx.Err() == nil {
		// A server that no longer knows previousResultID (it restarted) may
		// reject the delta; the editor would resend the same id forever.
		return nil, errFullNeeded
	}
	if err != nil {
		return nil, err
	}
	out, err := support.Remap(raw)
	if errors.Is(err, semtok.ErrMisalignedDelta) {
		return nil, errFullNeeded
	}
	return out, err
}

func (e *engine) SemanticTokensRange(
	ctx context.Context,
	wsID string,
	worktreePath string,
	filePath string,
	rng domlsp.Range,
) (json.RawMessage, error) {
	params := convert.SemanticTokensRangeParams(absFilePath(worktreePath, filePath), rng)
	var out json.RawMessage
	_, err := e.serve(ctx, wsID, worktreePath, filePath, func(reqCtx context.Context, srv server.Server) error {
		support := srv.SemanticTokens()
		if !support.Range() {
			return nil
		}
		raw, err := srv.Request(reqCtx, "textDocument/semanticTokens/range", params)
		if err != nil {
			return err
		}
		out, err = support.Remap(raw)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("lsp: semantic tokens range: %w", err)
	}
	return out, nil
}
