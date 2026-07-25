package manager

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
)

// A language server publishes diagnostics against the absolute file URIs it was
// given, but the editor addresses files by workspace-relative path (the same
// format the files API takes). The manager knows the worktree a server was
// spawned in, so it relativizes each diagnostic's FilePath on the way out,
// exactly as the engine does for definition/references/rename results. A path
// outside the worktree cannot be expressed relative to it and stays absolute.
func diagnosticsFor(
	t *testing.T,
	worktreePath string,
	emitted []domlsp.Diagnostic,
) []domlsp.Diagnostic {
	t.Helper()

	var mu sync.Mutex
	var fakes []*fakeServer
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		fs := &fakeServer{}
		mu.Lock()
		fakes = append(fakes, fs)
		mu.Unlock()
		return fs, nil
	}

	m := New(registry.New(nil), spawn, foundLookPath())

	var received []domlsp.DiagnosticsEvent
	m.OnDiagnostics(func(
		e domlsp.DiagnosticsEvent,
	) {
		received = append(received, e)
	})

	_, err := m.ServerForFile(context.Background(), "ws1", worktreePath, "main.go")
	require.NoError(t, err)

	mu.Lock()
	require.Len(t, fakes, 1)
	fake := fakes[0]
	mu.Unlock()

	fake.mu.Lock()
	emit := fake.diagFn
	fake.mu.Unlock()
	require.NotNil(t, emit, "OnDiagnostics must have been wired on the spawned server")

	emit(domlsp.DiagnosticsEvent{Diagnostics: emitted})

	require.Len(t, received, 1)
	return received[0].Diagnostics
}

func TestOnDiagnostics_RelativizesFilePathsToTheWorktree(t *testing.T) {
	got := diagnosticsFor(t, "/repo", []domlsp.Diagnostic{
		{FilePath: "/repo/pkg/util.go", Message: "boom"},
		{FilePath: "/repo/main.go", Message: "bang"},
	})

	require.Len(t, got, 2)
	assert.Equal(t, "pkg/util.go", got[0].FilePath)
	assert.Equal(t, "main.go", got[1].FilePath)
}

func TestOnDiagnostics_KeepsPathsOutsideTheWorktreeAbsolute(t *testing.T) {
	got := diagnosticsFor(t, "/repo", []domlsp.Diagnostic{
		{FilePath: "/usr/local/go/src/fmt/print.go", Message: "boom"},
	})

	require.Len(t, got, 1)
	assert.Equal(t, "/usr/local/go/src/fmt/print.go", got[0].FilePath)
}
