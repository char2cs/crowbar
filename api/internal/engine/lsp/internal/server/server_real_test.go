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

	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
)

func TestRealGopls(t *testing.T) {
	goplsPath, err := exec.LookPath("gopls")
	if err != nil {
		t.Skipf("gopls not found in PATH: %v", err)
	}
	t.Logf("gopls found at %s", goplsPath)

	dir := scaffoldGoModule(t)

	ctx, cancel := context.WithTimeout(context.Background(), 25_000_000_000)
	t.Cleanup(cancel)

	srv, err := New(ctx, "gopls", []string{}, dir, map[string]any{"semanticTokens": true})
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

func TestRealGopls_SemanticTokens(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skipf("gopls not found in PATH: %v", err)
	}
	dir := scaffoldGoModule(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25_000_000_000)
	t.Cleanup(cancel)

	srv, err := New(ctx, "gopls", []string{}, dir, map[string]any{"semanticTokens": true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })
	require.NoError(t, srv.Initialize(ctx, dir))

	support := srv.SemanticTokens()
	require.True(t, support.Full(), "gopls should advertise full semantic tokens")

	fileURI := fmt.Sprintf("file://%s", filepath.Join(dir, "main.go"))
	notifyDidOpen(t, ctx, srv, fileURI, mainGoSource())
	raw, err := srv.Request(ctx, "textDocument/semanticTokens/full",
		map[string]any{"textDocument": map[string]any{"uri": fileURI}})
	require.NoError(t, err)
	remapped, err := support.Remap(raw)
	require.NoError(t, err)

	var tokens struct {
		Data []uint32 `json:"data"`
	}
	require.NoError(t, json.Unmarshal(remapped, &tokens))
	require.NotEmpty(t, tokens.Data)
	types := map[string]bool{}
	for i := 3; i < len(tokens.Data); i += 5 {
		if int(tokens.Data[i]) < len(semtok.TokenTypes) {
			types[semtok.TokenTypes[tokens.Data[i]]] = true
		}
	}
	assert.True(t, types["function"], "Hello is a function token: %v", types)
	assert.True(t, types["type"], "Greeting is a type token: %v", types)
}

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
