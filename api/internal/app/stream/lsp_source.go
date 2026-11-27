package stream

import (
	"context"
	"encoding/json"
	"fmt"

	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type Diagnostics interface {
	DiagnosticsSnapshot(ownerID string) []lspdomain.Diagnostic
}

type LspSource struct {
	diagnostics Diagnostics
}

func NewLspSource(diagnostics Diagnostics) *LspSource {
	return &LspSource{diagnostics: diagnostics}
}

func (s *LspSource) Attach(_ context.Context, ownerID string, _ int64, emit func(stream.Frame)) error {
	diags := s.diagnostics.DiagnosticsSnapshot(ownerID)
	if len(diags) == 0 {
		return nil
	}
	data, err := json.Marshal(lspdomain.DiagnosticsEvent{WsID: ownerID, Diagnostics: diags})
	if err != nil {
		return fmt.Errorf("stream: lsp snapshot: %w", err)
	}
	emit(stream.Frame{T: stream.FrameSignal, Kind: stream.SignalDiagnostics, Data: data})
	return nil
}
