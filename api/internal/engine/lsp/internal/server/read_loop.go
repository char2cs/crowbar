package server

import (
	"bufio"
	"encoding/json"
	"io"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/convert"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/protocol"
)

func (s *server) readLoop(
	transport io.ReadWriteCloser,
	reader *bufio.Reader,
) {
	defer safego.Recover("lsp.server.readLoop")
	for {
		payload, err := protocol.ReadMessage(reader)
		if err != nil {
			return
		}
		s.dispatch(payload)
	}
}

func (s *server) dispatch(
	payload []byte,
) {
	// The id is read raw: a request FROM the server (id and method) may carry a
	// string id, while responses to our requests always carry our int ids.
	var msg struct {
		ID     json.RawMessage    `json:"id"`
		Method string             `json:"method"`
		Params json.RawMessage    `json:"params"`
		Result json.RawMessage    `json:"result"`
		Error  *protocol.RPCError `json:"error"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	hasID := len(msg.ID) > 0 && string(msg.ID) != "null"
	switch {
	case hasID && msg.Method != "":
		s.answer(msg.ID, msg.Method, msg.Params)
	case hasID:
		var id int
		if err := json.Unmarshal(msg.ID, &id); err != nil {
			return
		}
		s.deliver(id, protocol.Response{JSONRPC: "2.0", ID: &id, Result: msg.Result, Error: msg.Error})
	case msg.Method == methodPublishDiagnostics:
		s.handleDiagnostics(msg.Params)
	}
}

func (s *server) deliver(
	id int,
	resp protocol.Response,
) {
	s.mu.Lock()
	ch, ok := s.waiters[id]
	if ok {
		delete(s.waiters, id)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	ch <- waiterResult{resp: resp}
}

func (s *server) handleDiagnostics(
	params json.RawMessage,
) {
	s.mu.Lock()
	fn := s.onDiag
	s.mu.Unlock()
	if fn == nil {
		return
	}

	var p struct {
		URI         string                  `json:"uri"`
		Diagnostics []convert.LSPDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}

	diags := make([]lsp.Diagnostic, 0, len(p.Diagnostics))
	for _, raw := range p.Diagnostics {
		diags = append(diags, convert.DiagnosticFromLSP(p.URI, raw))
	}
	fn(lsp.DiagnosticsEvent{Diagnostics: diags})
}
