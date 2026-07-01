//go:build integration

package files_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

const wsTimeout = 5 * time.Second

func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

// FilesSuite is the integration test suite for the file usecase and the
// workspace-scoped files WebSocket topic. File writes target a writable child
// workspace (the adopted main worktree is locked, being a protected branch).
type FilesSuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
	wsID     string
}

func (s *FilesSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	repoPath := kit.InitRepoWithFile(s.T(), "hello.txt", "hello world\n")
	s.imported = s.Env.ImportRepo(s.T(), "files", repoPath)
	// A writable child forked from main inherits the committed hello.txt.
	s.wsID = s.Env.CreateWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/test-files")
}

// base returns the workspace-scoped route prefix for the writable child.
func (s *FilesSuite) base() string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/workspaces/" + s.wsID
}

// TestFilesSuite is the testify suite entry point for FilesSuite.
func TestFilesSuite(
	t *testing.T,
) {
	suite.Run(t, new(FilesSuite))
}

// nodeNames extracts the Name field from each node in the API response slice.
func nodeNames(
	nodes []map[string]any,
) []string {
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if name, ok := n["name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

// TestFiles_treeReturnsRootEntries verifies that Tree lists the committed file.
func (s *FilesSuite) TestFiles_treeReturnsRootEntries() {
	resp := s.Env.GET(s.T(), s.base()+"/files/tree?path=.")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)
	s.Assert().Contains(nodeNames(nodes), "hello.txt")
}

// TestFiles_readContentReturnsFileBytes verifies ReadContent returns the
// exact bytes committed to the repo.
func (s *FilesSuite) TestFiles_readContentReturnsFileBytes() {
	resp := s.Env.GET(s.T(), s.base()+"/files/content?path=hello.txt")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Assert().Equal("hello world\n", content["content"])
}

// TestFiles_writeContentMutatesAndResyncs verifies WriteContent persists to
// disk and triggers a SyncWorkingTreeState so git sees the dirty file.
func (s *FilesSuite) TestFiles_writeContentMutatesAndResyncs() {
	resp := s.Env.PUT(s.T(), s.base()+"/files/content", map[string]any{
		"path":    "hello.txt",
		"content": "updated content\n",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/files/content?path=hello.txt")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Assert().Equal("updated content\n", content["content"])
}

// TestFiles_createFileAppearsInTree verifies CreateFile makes the new path
// visible in the next Tree call.
func (s *FilesSuite) TestFiles_createFileAppearsInTree() {
	resp := s.Env.POST(s.T(), s.base()+"/files", map[string]any{
		"path": "new.txt",
		"type": "file",
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/files/tree?path=.")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)
	s.Assert().Contains(nodeNames(nodes), "new.txt")
}

// TestFiles_createDirAppearsInTree verifies CreateDir makes the directory
// visible in the next Tree call.
func (s *FilesSuite) TestFiles_createDirAppearsInTree() {
	resp := s.Env.POST(s.T(), s.base()+"/files", map[string]any{
		"path": "subdir",
		"type": "dir",
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/files/tree?path=.")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)
	s.Assert().Contains(nodeNames(nodes), "subdir")
}

// TestFiles_renameMovesPath verifies Rename makes old path disappear and new
// path appear in the tree.
func (s *FilesSuite) TestFiles_renameMovesPath() {
	resp := s.Env.PATCH(s.T(), s.base()+"/files", map[string]any{
		"path":    "hello.txt",
		"newPath": "renamed.txt",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/files/tree?path=.")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)
	names := nodeNames(nodes)
	s.Assert().Contains(names, "renamed.txt")
	s.Assert().NotContains(names, "hello.txt")
}

// TestFiles_deleteRemovesFromTree verifies Delete makes the path disappear
// from the next Tree call.
func (s *FilesSuite) TestFiles_deleteRemovesFromTree() {
	resp := s.Env.DELETE(s.T(), s.base()+"/files?path=hello.txt")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/files/tree?path=.")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)
	s.Assert().NotContains(nodeNames(nodes), "hello.txt")
}

// TestFiles_hubBroadcastReachesFilesWatcher verifies that a FileChangeEvent
// pushed directly to the hub is received by a connected files WS subscriber on
// the hierarchical .../files/ws route.
func (s *FilesSuite) TestFiles_hubBroadcastReachesFilesWatcher() {
	t := s.T()

	watcher := s.Env.DialFiles(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)
	s.Env.PushFile(kit.FileEvent{WsID: s.wsID})

	msg := watcher.ReadUntil(t, wsTimeout, func(m map[string]any) bool {
		wsID, _ := m["wsId"].(string)
		return wsID == s.wsID
	})
	s.Assert().Equal(s.wsID, msg["wsId"])
}
