//go:build lsp_real

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealGopls is a build-tagged smoke test that spawns the real gopls binary
// (if present) against a tiny temp Go module and verifies that
// textDocument/documentSymbol returns the expected symbols.
//
// The test is excluded from the default suite and NEVER fails on a machine
// where gopls is absent — it skips instead.
func TestRealGopls(t *testing.T) {
	goplsPath, err := exec.LookPath("gopls")
	if err != nil {
		t.Skipf("gopls not found in PATH: %v", err)
	}
	t.Logf("gopls found at %s", goplsPath)

	dir := scaffoldGoModule(t)

	ctx, cancel := context.WithTimeout(context.Background(), 25_000_000_000) // 25 s
	t.Cleanup(cancel)

	srv, err := New("gopls", []string{}, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	doInitialize(t, ctx, srv, dir)

	fileURI := fmt.Sprintf("file://%s", filepath.Join(dir, "main.go"))
	src := mainGoSource()

	notifyDidOpen(t, ctx, srv, fileURI, src)

	symbols := requestDocumentSymbol(t, ctx, srv, fileURI)

	names := symbolNames(symbols)
	assert.Contains(t, names, "Hello", "expected Hello function in symbols")
	assert.Contains(t, names, "Greeting", "expected Greeting type in symbols")
}

// scaffoldGoModule creates a temp directory with a minimal Go module and a
// source file containing a couple of named symbols.
func scaffoldGoModule(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()

	goMod := `module crowbar.test/smoke

go 1.21
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGoSource()), 0o644))
	return dir
}

func mainGoSource() string {
	return `package main

// Greeting is a named string type used as a smoke-test symbol.
type Greeting string

// Hello returns a Greeting.
func Hello() Greeting {
	return Greeting("hello")
}

func main() {}
`
}

// doInitialize performs the LSP initialize / initialized handshake required
// before any workspace feature requests.
func doInitialize(
	t *testing.T,
	ctx context.Context,
	srv Server,
	rootDir string,
) {
	t.Helper()

	rootURI := fmt.Sprintf("file://%s", rootDir)
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"documentSymbol": map[string]any{
					"hierarchicalDocumentSymbolSupport": false,
				},
			},
		},
	}

	_, err := srv.Request(ctx, "initialize", params)
	require.NoError(t, err, "initialize failed")

	err = srv.Notify(ctx, "initialized", map[string]any{})
	require.NoError(t, err, "initialized notification failed")
}

// notifyDidOpen sends textDocument/didOpen so gopls starts tracking the file.
func notifyDidOpen(
	t *testing.T,
	ctx context.Context,
	srv Server,
	fileURI string,
	src string,
) {
	t.Helper()

	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        fileURI,
			"languageId": "go",
			"version":    1,
			"text":       src,
		},
	}
	require.NoError(t, srv.Notify(ctx, "textDocument/didOpen", params))
}

// requestDocumentSymbol sends textDocument/documentSymbol and parses the flat
// SymbolInformation[] result (hierarchicalDocumentSymbolSupport: false).
func requestDocumentSymbol(
	t *testing.T,
	ctx context.Context,
	srv Server,
	fileURI string,
) []map[string]any {
	t.Helper()

	params := map[string]any{
		"textDocument": map[string]any{"uri": fileURI},
	}

	raw, err := srv.Request(ctx, "textDocument/documentSymbol", params)
	require.NoError(t, err, "documentSymbol request failed")

	var symbols []map[string]any
	require.NoError(t, json.Unmarshal(raw, &symbols), "failed to parse documentSymbol response")
	return symbols
}

// symbolNames extracts the "name" field from each symbol in the list.
func symbolNames(
	symbols []map[string]any,
) []string {
	names := make([]string, 0, len(symbols))
	for _, s := range symbols {
		if name, ok := s["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}
