package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/manager"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/server"
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

func TestCompletion_ForwardsAndPassesThrough(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"items":[{"label":"Foo"}]}`))
	e := buildEngine(t, fake)

	got, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[{"label":"Foo"}]}`, string(got))

	reqs := fake.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "textDocument/completion", reqs[0].method)
}

func TestHover_Forwards(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"contents":"doc"}`))
	e := buildEngine(t, fake)

	got, err := e.Hover(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)
	assert.JSONEq(t, `{"contents":"doc"}`, string(got))
	assert.Equal(t, "textDocument/hover", fake.requests()[0].method)
}

func TestDefinition_ConvertsLocations(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`[{"uri":"file:///tree/a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}}]`,
	))
	e := buildEngine(t, fake)

	locs, err := e.Definition(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)
	require.Len(t, locs, 1)
	assert.Equal(t, "a.go", locs[0].FilePath)
	assert.Equal(t, "textDocument/definition", fake.requests()[0].method)
}

func TestReferences_ConvertsLocations(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`[{"uri":"file:///tree/a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}}},
		  {"uri":"file:///tree/b.go","range":{"start":{"line":2,"character":1},"end":{"line":2,"character":3}}}]`,
	))
	e := buildEngine(t, fake)

	locs, err := e.References(context.Background(), ws, tree, goF, pos())
	require.NoError(t, err)
	require.Len(t, locs, 2)
	assert.Equal(t, "textDocument/references", fake.requests()[0].method)
}

func TestRename_ConvertsWorkspaceEdit(t *testing.T) {
	fake := newFakeServer(json.RawMessage(
		`{"changes":{"file:///tree/a.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":3}},"newText":"X"}]}}`,
	))
	e := buildEngine(t, fake)

	we, err := e.Rename(context.Background(), ws, tree, goF, pos(), "X")
	require.NoError(t, err)
	edits, ok := we.Changes["a.go"]
	require.True(t, ok)
	require.Len(t, edits, 1)
	assert.Equal(t, "X", edits[0].NewText)

	reqs := fake.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "textDocument/rename", reqs[0].method)
}

func TestCodeAction_Forwards(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`[{"title":"Fix","edit":{"changes":{}}}]`))
	e := buildEngine(t, fake)

	rng := domlsp.Range{Start: domlsp.Position{Line: 1}, End: domlsp.Position{Line: 2}}
	got, err := e.CodeAction(context.Background(), ws, tree, goF, rng, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"title":"Fix","edit":{"changes":{}}}]`, string(got))
	assert.Equal(t, "textDocument/codeAction", fake.requests()[0].method)
}

func TestCodeAction_ForwardsDiagnosticsAndRelativizesEdits(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`[
		{"title":"Fix","kind":"quickfix","edit":{"changes":{"file:///tree/pkg/a.go":[
			{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"newText":"x"}
		]}}},
		{"title":"Organize","edit":{"documentChanges":[
			{"textDocument":{"uri":"file:///tree/main.go","version":3},"edits":[]}
		]}},
		{"title":"Run","command":"gopls.run"},
		{"title":"Show","command":"java.show.references"},
		{"title":"Both","edit":{"changes":{}},"command":{"title":"x","command":"client.only"}}
	]`))
	fake.commands = map[string]bool{"gopls.run": true}
	e := buildEngine(t, fake)
	diags := json.RawMessage(`[{"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":4}},"message":"unused"}]`)

	got, err := e.CodeAction(context.Background(), ws, tree, goF, domlsp.Range{}, diags)
	require.NoError(t, err)

	params := as[map[string]any](t, fake.requests()[0].params)
	ctxParam := as[map[string]any](t, params["context"])
	assert.JSONEq(t, string(diags), string(as[json.RawMessage](t, ctxParam["diagnostics"])))

	assert.JSONEq(t, `[
		{"title":"Fix","kind":"quickfix","edit":{"changes":{"pkg/a.go":[
			{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}},"newText":"x"}
		]}}},
		{"title":"Organize","edit":{"documentChanges":[
			{"textDocument":{"uri":"main.go","version":3},"edits":[]}
		]}},
		{"title":"Run","command":"gopls.run"},
		{"title":"Both","edit":{"changes":{}}}
	]`, string(got))
}

func TestNewFeatureRequests_ForwardTheirMethods(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"ok":true}`))
	e := buildEngine(t, fake)
	ctx := context.Background()

	_, err := e.SignatureHelp(ctx, ws, tree, goF, pos())
	require.NoError(t, err)
	_, err = e.CodeLens(ctx, ws, tree, goF)
	require.NoError(t, err)
	_, err = e.CodeLensResolve(ctx, ws, tree, goF, json.RawMessage(`{"range":{}}`))
	require.NoError(t, err)
	_, err = e.Formatting(ctx, ws, tree, goF, domlsp.FormattingOptions{TabSize: 4, InsertSpaces: true})
	require.NoError(t, err)

	reqs := fake.requests()
	require.Len(t, reqs, 4)
	assert.Equal(t, "textDocument/signatureHelp", reqs[0].method)
	assert.Equal(t, "textDocument/codeLens", reqs[1].method)
	assert.Equal(t, "codeLens/resolve", reqs[2].method)
	assert.Equal(t, "textDocument/formatting", reqs[3].method)
	opts := as[map[string]any](t, as[map[string]any](t, reqs[3].params)["options"])
	assert.Equal(t, 4, opts["tabSize"])
	assert.Equal(t, true, opts["insertSpaces"])
}

func TestDocumentSymbol_Forwards(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`[{"name":"Foo","kind":12}]`))
	e := buildEngine(t, fake)

	got, err := e.DocumentSymbol(context.Background(), ws, tree, goF)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"Foo","kind":12}]`, string(got))
	assert.Equal(t, "textDocument/documentSymbol", fake.requests()[0].method)
}

func TestAbsFilePath(t *testing.T) {
	assert.Equal(t, "/tree/main.go", absFilePath("/tree", "main.go"))
	assert.Equal(t, "/tree/pkg/util.go", absFilePath("/tree", "pkg/util.go"))
	assert.Equal(t, "/already/abs.go", absFilePath("/tree", "/already/abs.go"))
}

// TestEngine_FeatureRequestsAreRefNeutral proves a feature request with NO open
// document is net-zero: each request spawns the server, serves, and tears it
// down. Before the fix the refcount grew by one per request and the server was
// never closed.
func TestEngine_FeatureRequestsAreRefNeutral(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"items":[]}`))
	e, sc := buildCountingEngine(t, fake)
	ctx := context.Background()

	const n = 4
	for i := 0; i < n; i++ {
		_, err := e.Completion(ctx, ws, tree, goF, pos())
		require.NoError(t, err)
	}

	// No open doc: each request must balance — spawn then teardown — so the
	// counts grow together and the pool never retains a leaked ref.
	assert.Equal(t, n, sc.count(), "each request spawns a fresh warm server")
	assert.Equal(t, n, fake.closeCount(), "each request tears the server down again")
}

func TestRequest_ServerErrorPropagates(t *testing.T) {
	fake := newFakeServer(nil)
	fake.reqErr = errors.New("boom")
	e := buildEngine(t, fake)

	_, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: textDocument/completion")
}

func TestRequest_SpawnErrorPropagates(t *testing.T) {
	e := buildFailingEngine(t)
	_, err := e.Definition(context.Background(), ws, tree, goF, pos())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lsp: textDocument/definition")
}

func TestRequest_TimesOutOnWedgedServer(t *testing.T) {
	fake := &blockingServer{fakeServer: newFakeServer(nil)}
	reg := registry.New(nil)
	spawn := func(
		_ context.Context,
		_ registry.ServerSpec,
		_ string,
	) (server.Server, error) {
		return fake, nil
	}
	e := newWithManager(reg, manager.New(reg, spawn, foundLookPath()))
	e.reqTimeout = time.Millisecond

	_, err := e.Completion(context.Background(), ws, tree, goF, pos())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
