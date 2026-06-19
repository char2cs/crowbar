package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TestWorkspacesDef_Lambdas(t *testing.T) {
	def := workspacesDef(nil)
	d := dto.WorkspaceDTO{ID: "w1", ProjectID: "p1", RepoID: "r1"}

	assert.Equal(t, "p1/r1/w1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "w1")

	// Prefix-based scoping replaces the projectId/repoId query Filters (spec §5).
	assert.Empty(t, def.Filters)
}

func TestProjectsDef_NamespaceID(t *testing.T) {
	def := projectsDef(nil)
	d := dto.ProjectDTO{ID: "p1"}

	assert.Equal(t, "p1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "p1")
}

func TestReposDef_NamespaceProjectRepo(t *testing.T) {
	def := reposDef(nil)
	d := dto.RepoDTO{ID: "r1", ProjectID: "p1"}

	assert.Equal(t, "p1/r1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "r1")
}

func TestThreadsDef_NamespaceProjectRepoWsID(t *testing.T) {
	def := threadsDef()
	d := dto.ThreadDTO{ID: "t1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"}

	assert.Equal(t, "p1/r1/w1/t1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "t1")
}

func TestTerminalsDef_NamespaceProjectRepoWs(t *testing.T) {
	def := terminalsDef(nil, nil)
	d := dto.TerminalSessionDTO{ID: "s1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"}

	// The namespace is the workspace prefix (p/r/w), NOT the session leaf: a
	// workspace-scoped subscription receives every session in that workspace.
	assert.Equal(t, "p1/r1/w1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "s1")
}

func TestTerminalsDef_FiltersScopeByProjectRepoWs(t *testing.T) {
	def := terminalsDef(nil, nil)
	d := dto.TerminalSessionDTO{ID: "s1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"}

	require.Len(t, def.Filters, 3)
	assert.Equal(t, "projectId", def.Filters[0].Param)
	assert.Equal(t, "p1", def.Filters[0].Extract(d))
	assert.Equal(t, "repoId", def.Filters[1].Param)
	assert.Equal(t, "r1", def.Filters[1].Extract(d))
	assert.Equal(t, "wsId", def.Filters[2].Param)
	assert.Equal(t, "w1", def.Filters[2].Extract(d))
}

func TestTerminalsDef_SnapshotNilWithoutEngine(t *testing.T) {
	def := terminalsDef(nil, nil)
	assert.Nil(t, def.Snapshot)
}

func TestGitDef_Lambdas(t *testing.T) {
	def := gitDef(nil)
	evt := gitdomain.GitStatusEvent{
		WsID:   "w1",
		Status: gitdomain.GitStatus{Branch: "main"},
	}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "main")
	assert.NotContains(t, string(data), "wsId")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}

func TestFilesDef_Lambdas(t *testing.T) {
	def := filesDef()
	evt := domain.FileChangeEvent{WsID: "w1", Path: "a.go"}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "a.go")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}

func TestLSPDef_Lambdas(t *testing.T) {
	def := lspDef(nil, nil)
	evt := lspdomain.DiagnosticsEvent{WsID: "w1"}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "w1")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}
