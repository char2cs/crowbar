package tree_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/tree"
)

type noopProvider struct{}

func (n *noopProvider) GitStatus(
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

type fakeProvider struct {
	files []gitdomain.GitFile
}

func (f *fakeProvider) GitStatus(
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{Files: f.files}, nil
}

// errorProvider simulates a StatusProvider that always returns an error.
type errorProvider struct{}

func (e *errorProvider) GitStatus(
	_ string,
) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, errors.New("git status unavailable")
}

func TestList_Root(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), nil, 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o700))

	nodes, err := tree.List(dir, "", &noopProvider{})
	require.NoError(t, err)

	assert.Len(t, nodes, 3)
	assert.Equal(t, domain.FileNodeTypeDirectory, nodes[0].Type)
	assert.Equal(t, "subdir", nodes[0].Name)
}

func TestList_SkipsGitDir(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), nil, 0o600))

	nodes, err := tree.List(dir, "", &noopProvider{})
	require.NoError(t, err)

	for _, n := range nodes {
		assert.NotEqual(t, ".git", n.Name)
	}
}

func TestList_GitDecoration(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "modified.txt"), nil, 0o600))

	fp := &fakeProvider{files: []gitdomain.GitFile{
		{Path: "modified.txt", Status: gitdomain.GitFileStatusModified, Staged: false},
	}}

	nodes, err := tree.List(dir, "", fp)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.NotNil(t, nodes[0].GitStatus)
	assert.Equal(t, domain.FileNodeGitStatusModified, *nodes[0].GitStatus)
}

func TestList_ConflictedCollapsesToModified(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), nil, 0o600))

	fp := &fakeProvider{files: []gitdomain.GitFile{
		{Path: "conflict.txt", Status: gitdomain.GitFileStatusConflicted},
	}}

	nodes, err := tree.List(dir, "", fp)
	require.NoError(t, err)
	require.NotNil(t, nodes[0].GitStatus)
	assert.Equal(t, domain.FileNodeGitStatusModified, *nodes[0].GitStatus)
}

func TestList_Subdir(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub/deep.txt"), nil, 0o600))

	nodes, err := tree.List(dir, "sub", &noopProvider{})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "sub/deep.txt", nodes[0].Path)
}

func TestList_NilProvider(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), nil, 0o600))

	nodes, err := tree.List(dir, "", nil)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Nil(t, nodes[0].GitStatus)
}

// TestList_InvalidDir exercises the os.ReadDir error path.
func TestList_InvalidDir(
	t *testing.T,
) {
	_, err := tree.List("/nonexistent/path/abc", "", &noopProvider{})
	require.Error(t, err)
}

// TestList_ProviderError verifies that a StatusProvider error is silently
// swallowed (buildStatusMap returns nil) and List still succeeds.
func TestList_ProviderError(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), nil, 0o600))

	nodes, err := tree.List(dir, "", &errorProvider{})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Nil(t, nodes[0].GitStatus)
}

// TestList_DirsBeforeFiles verifies the sort: directories first, then files,
// all alphabetically within each group.
func TestList_DirsBeforeFiles(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "z.txt"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "m-dir"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a-dir"), 0o700))

	nodes, err := tree.List(dir, "", &noopProvider{})
	require.NoError(t, err)
	require.Len(t, nodes, 4)

	assert.Equal(t, domain.FileNodeTypeDirectory, nodes[0].Type)
	assert.Equal(t, "a-dir", nodes[0].Name)
	assert.Equal(t, domain.FileNodeTypeDirectory, nodes[1].Type)
	assert.Equal(t, "m-dir", nodes[1].Name)
	assert.Equal(t, domain.FileNodeTypeFile, nodes[2].Type)
	assert.Equal(t, "a.txt", nodes[2].Name)
	assert.Equal(t, domain.FileNodeTypeFile, nodes[3].Type)
	assert.Equal(t, "z.txt", nodes[3].Name)
}

// TestList_AllGitDecorations exercises every gitDecoration branch by providing
// files with each possible GitFileStatus value.
func TestList_AllGitDecorations(
	t *testing.T,
) {
	dir := t.TempDir()

	files := []struct {
		name        string
		status      gitdomain.GitFileStatus
		expectedDec domain.FileNodeGitStatus
	}{
		{"added.txt", gitdomain.GitFileStatusAdded, domain.FileNodeGitStatusAdded},
		{"deleted.txt", gitdomain.GitFileStatusDeleted, domain.FileNodeGitStatusDeleted},
		{"untracked.txt", gitdomain.GitFileStatusUntracked, domain.FileNodeGitStatusUntracked},
		{"renamed.txt", gitdomain.GitFileStatusRenamed, domain.FileNodeGitStatusRenamed},
	}

	gitFiles := make([]gitdomain.GitFile, 0, len(files))
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.name), nil, 0o600))
		gitFiles = append(gitFiles, gitdomain.GitFile{Path: f.name, Status: f.status})
	}

	fp := &fakeProvider{files: gitFiles}
	nodes, err := tree.List(dir, "", fp)
	require.NoError(t, err)

	nodeMap := make(map[string]domain.FileNodeGitStatus)
	for _, n := range nodes {
		if n.GitStatus != nil {
			nodeMap[n.Name] = *n.GitStatus
		}
	}

	for _, f := range files {
		dec, ok := nodeMap[f.name]
		require.True(t, ok, "expected decoration for %s", f.name)
		assert.Equal(t, f.expectedDec, dec)
	}
}

// TestList_NestedSubdirs verifies that listing a subdirectory that itself
// contains further nested subdirectories returns only one level.
func TestList_NestedSubdirs(
	t *testing.T,
) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "parent/child/grandchild"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parent/file.txt"), nil, 0o600))

	nodes, err := tree.List(dir, "parent", &noopProvider{})
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	// child dir first, then file
	assert.Equal(t, domain.FileNodeTypeDirectory, nodes[0].Type)
	assert.Equal(t, "child", nodes[0].Name)
	assert.Equal(t, domain.FileNodeTypeFile, nodes[1].Type)
	assert.Equal(t, "file.txt", nodes[1].Name)
}
