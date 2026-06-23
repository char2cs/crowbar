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
	var resp protocol.Response
	if err := json.Unmarshal(payload, &resp); err != nil {
		return
	}
	if resp.ID != nil {
		s.deliver(*resp.ID, resp)
		return
	}
	if resp.Method == methodPublishDiagnostics {
		s.handleDiagnostics(resp.Params)
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
