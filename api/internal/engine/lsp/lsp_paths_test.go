package lsp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The LSP path contract has two halves and the engine owns both. Requests
// arrive from the frontend with WORKSPACE-RELATIVE paths (the same format the
// files API and the editor's buffers use) and must reach the language server as
// ABSOLUTE file URIs rooted at the worktree, matching the URIs didOpen already
// publishes. Results travel the opposite way: a server answers with absolute
// file URIs and the frontend can only act on workspace-relative paths, so the
// engine relativizes them back before the API layer serializes them.

const relGoF = "pkg/util.go"

func requestParams(
	t *testing.T,
	fake *fakeServer,
) map[string]any {
	t.Helper()
	reqs := fake.requests()
	require.Len(t, reqs, 1)
	params, ok := reqs[0].params.(map[string]any)
	require.True(t, ok, "params must be the LSP parameter object")
	return params
}

func documentURI(
	t *testing.T,
	fake *fakeServer,
) string {
	t.Helper()
	doc, ok := requestParams(t, fake)["textDocument"].(map[string]any)
	require.True(t, ok, "params must carry a textDocument")
	uri, ok := doc["uri"].(string)
	require.True(t, ok, "textDocument must carry a uri")
	return uri
}

func TestDefinition_RequestsAbsoluteDocumentURI(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`null`))
	e := buildEngine(t, fake)

	_, err := e.Definition(context.Background(), ws, tree, relGoF, pos())
	require.NoError(t, err)
	assert.Equal(t, "file:///tree/pkg/util.go", documentURI(t, fake))
}

func TestReferences_RequestsAbsoluteDocumentURI(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`null`))
	e := buildEngine(t, fake)

	_, err := e.References(context.Background(), ws, tree, relGoF, pos())
	require.NoError(t, err)
	assert.Equal(t, "file:///tree/pkg/util.go", documentURI(t, fake))
}

func TestCompletion_RequestsAbsoluteDocumentURI(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`null`))
	e := buildEngine(t, fake)

	_, err := e.Completion(context.Background(), ws, tree, relGoF, pos())
	require.NoError(t, err)
	assert.Equal(t, "file:///tree/pkg/util.go", documentURI(t, fake))
}

func TestHover_RequestsAbsoluteDocumentURI(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`null`))
	e := buildEngine(t, fake)

	_, err := e.Hover(context.Background(), ws, tree, relGoF, pos())
	require.NoError(t, err)
	assert.Equal(t, "file:///tree/pkg/util.go", documentURI(t, fake))
}

func TestRename_RequestsAbsoluteDocumentURI(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`null`))
	e := buildEngine(t, fake)

	_, err := e.Rename(context.Background(), ws, tree, relGoF, pos(), "X")
	require.NoError(t, err)
	assert.Equal(t, "file:///tree/pkg/util.go", documentURI(t, fake))
}

func TestDocumentSymbol_RequestsAbsoluteDocumentURI(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`null`))
	e := buildEngine(t, fake)

	_, err := e.DocumentSymbol(context.Background(), ws, tree, relGoF)
	require.NoError(t, err)
	assert.Equal(t, "file:///tree/pkg/util.go", documentURI(t, fake))
}

func TestDefinition_ReturnsWorkspaceRelativePaths(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`[{"uri":"file:///tree/pkg/target.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}}]`,
	))
	e := buildEngine(t, fake)

	locs, err := e.Definition(context.Background(), ws, tree, relGoF, pos())
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Equal(t, "pkg/target.go", locs[0].FilePath)
}

func TestDefinition_KeepsTargetsOutsideTheWorktreeAbsolute(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`[{"uri":"file:///usr/local/go/src/fmt/print.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}}]`,
	))
	e := buildEngine(t, fake)

	locs, err := e.Definition(context.Background(), ws, tree, relGoF, pos())
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Equal(t, "/usr/local/go/src/fmt/print.go", locs[0].FilePath)
}

func TestReferences_ReturnsWorkspaceRelativePaths(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`[{"uri":"file:///tree/a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}},
		  {"uri":"file:///tree/pkg/b.go","range":{"start":{"line":2,"character":1},"end":{"line":2,"character":3}}}]`,
	))
	e := buildEngine(t, fake)

	locs, err := e.References(context.Background(), ws, tree, relGoF, pos())
	require.NoError(t, err)
	require.Len(t, locs, 2)
	assert.Equal(t, "a.go", locs[0].FilePath)
	assert.Equal(t, "pkg/b.go", locs[1].FilePath)
}

func TestRename_ReturnsWorkspaceRelativeChangeKeys(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`{"changes":{"file:///tree/pkg/a.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":3}},"newText":"X"}]}}`,
	))
	e := buildEngine(t, fake)

	we, err := e.Rename(context.Background(), ws, tree, relGoF, pos(), "X")
	require.NoError(t, err)
	edits, ok := we.Changes["pkg/a.go"]
	require.True(t, ok, "change keys must be workspace-relative")
	require.Len(t, edits, 1)
	assert.Equal(t, "X", edits[0].NewText)
}
