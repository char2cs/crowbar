package stream_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type latestDiagnostics map[string][]lspdomain.Diagnostic

func (l latestDiagnostics) DiagnosticsSnapshot(ownerID string) []lspdomain.Diagnostic {
	return l[ownerID]
}

func TestLspSource_SnapshotsTheOwnersDiagnostics(t *testing.T) {
	src := appstream.NewLspSource(latestDiagnostics{"c1": {{Message: "unused"}}, "c2": {{Message: "other"}}})
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindLsp: appstream.Unordered(src)}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	frames := conn.sub(t, "lsp:c1")
	require.Len(t, frames, 1)
	assert.Equal(t, stream.FrameSignal, frames[0].T)
	assert.Equal(t, "diagnostics", frames[0].Kind)
	assert.Contains(t, string(frames[0].Data), `"wsId":"c1"`)
	assert.Contains(t, string(frames[0].Data), "unused")

	assert.Empty(t, conn.sub(t, "lsp:c9"), "an owner with no diagnostics snapshots nothing")
}
