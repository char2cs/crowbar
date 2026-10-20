package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/protocol"
)

const (
	// errMethodNotFound is JSON-RPC's code for a request the client cannot serve.
	errMethodNotFound = -32601
	// errInvalidParams is JSON-RPC's code for params the client cannot decode.
	errInvalidParams = -32602
)

// answer replies to a request the server sent to the client. A server blocks on
// these (gopls waits on workspace/applyEdit before answering the command that
// caused it), so every one gets a reply. Any state is recorded here, on the
// read loop, before the reply exists; the write itself runs off the read loop,
// which must keep draining the server's stdout or both sides stall on full
// pipes.
func (s *server) answer(
	id json.RawMessage,
	method string,
	params json.RawMessage,
) {
	reply := protocol.Reply{JSONRPC: "2.0", ID: id, Result: json.RawMessage("null")}
	var paramsErr error
	switch method {
	case "workspace/applyEdit":
		reply.Result, paramsErr = s.collectEdit(params)
	case "workspace/configuration":
		reply.Result, paramsErr = emptyConfiguration(params)
	case "client/registerCapability", "client/unregisterCapability",
		"window/workDoneProgress/create", "window/showMessageRequest":
	default:
		reply.Result = nil
		reply.Error = &protocol.RPCError{Code: errMethodNotFound, Message: "unsupported: " + method}
	}
	if paramsErr != nil {
		reply.Result = nil
		reply.Error = &protocol.RPCError{Code: errInvalidParams, Message: paramsErr.Error()}
	}
	safego.Go("lsp.server.answer", func() {
		if err := s.write(reply); err != nil && !errors.Is(err, ErrClosed) {
			slog.Warn("lsp: answer server request", "method", method, "err", err)
		}
	})
}

// collectEdit hands a workspace/applyEdit to the command that is running, so
// the editor applies it; with no command running there is nobody to apply it.
func (s *server) collectEdit(
	params json.RawMessage,
) (json.RawMessage, error) {
	var p struct {
		Edit json.RawMessage `json:"edit"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("workspace/applyEdit params: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commandEdits == nil || len(p.Edit) == 0 {
		return json.RawMessage(`{"applied":false,"failureReason":"no command is running"}`), nil
	}
	*s.commandEdits = append(*s.commandEdits, p.Edit)
	return json.RawMessage(`{"applied":true}`), nil
}

// emptyConfiguration answers workspace/configuration with one null per item:
// "no client-side setting", so the server keeps its defaults.
func emptyConfiguration(
	params json.RawMessage,
) (json.RawMessage, error) {
	var p struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("workspace/configuration params: %w", err)
	}
	nulls := strings.Join(slices.Repeat([]string{"null"}, len(p.Items)), ",")
	return json.RawMessage("[" + nulls + "]"), nil
}

func (s *server) ExecuteCommand(
	ctx context.Context,
	params any,
) (json.RawMessage, []json.RawMessage, error) {
	s.execMu.Lock()
	defer s.execMu.Unlock()
	edits := []json.RawMessage{}
	s.mu.Lock()
	s.commandEdits = &edits
	s.mu.Unlock()

	result, err := s.Request(ctx, "workspace/executeCommand", params)

	s.mu.Lock()
	s.commandEdits = nil
	collected := edits
	s.mu.Unlock()
	if err != nil {
		return nil, nil, fmt.Errorf("execute command: %w", err)
	}
	return result, collected, nil
}
