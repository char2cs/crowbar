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

// FilesSuite is the integration test suite for the file usecase and /v0/ws/files WebSocket topic.
type FilesSuite struct {
	kit.IntegrationSuite
	repoPath string
	wsID     string
}

func (s *FilesSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()

	s.repoPath = kit.InitRepoWithFile(
		s.T(),
		"hello.txt",
		"hello world\n",
	)

	kit.GitRun(s.T(), s.repoPath, "branch", "-m", "main", "feature/test-files")

	branch := kit.BranchName(
		s.T(),
		s.repoPath,
	)

	repoResp := s.Env.POST(s.T(), "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      s.repoPath,
	})
	kit.RequireStatus(s.T(), repoResp, http.StatusCreated)
	repoResp.Body.Close()

	resp := s.Env.POST(
		s.T(),
		"/v0/workspaces",
		map[string]any{
			"repoId": "r1",
			"branch": branch,
		},
	)
	kit.RequireStatus(s.T(), resp, 201)
	s.wsID = kit.MutationID(s.T(), resp)
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
	resp := s.Env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files/tree?path=.",
	)
	kit.RequireStatus(s.T(), resp, 200)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)

	s.Assert().Contains(
		nodeNames(nodes),
		"hello.txt",
	)
}

// TestFiles_readContentReturnsFileBytes verifies ReadContent returns the
// exact bytes committed to the repo.
func (s *FilesSuite) TestFiles_readContentReturnsFileBytes() {
	resp := s.Env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files/content?path=hello.txt",
	)
	kit.RequireStatus(s.T(), resp, 200)

	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)

	s.Assert().Equal(
		"hello world\n",
		content["content"],
	)
}

// TestFiles_writeContentMutatesAndResyncs verifies WriteContent persists to
// disk and triggers a SyncWorkingTreeState so git sees the dirty file.
func (s *FilesSuite) TestFiles_writeContentMutatesAndResyncs() {
	resp := s.Env.PUT(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files/content",
		map[string]any{
			"path":    "hello.txt",
			"content": "updated content\n",
		},
	)
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	resp = s.Env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files/content?path=hello.txt",
	)
	kit.RequireStatus(s.T(), resp, 200)

	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)

	s.Assert().Equal(
		"updated content\n",
		content["content"],
	)
}

// TestFiles_createFileAppearsInTree verifies CreateFile makes the new path
// visible in the next Tree call.
func (s *FilesSuite) TestFiles_createFileAppearsInTree() {
	resp := s.Env.POST(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files",
		map[string]any{
			"path": "new.txt",
			"type": "file",
		},
	)
	kit.RequireStatus(s.T(), resp, 201)
	resp.Body.Close()

	resp = s.Env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files/tree?path=.",
	)
	kit.RequireStatus(s.T(), resp, 200)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)

	s.Assert().Contains(
		nodeNames(nodes),
		"new.txt",
	)
}

// TestFiles_createDirAppearsInTree verifies CreateDir makes the directory
// visible in the next Tree call.
func (s *FilesSuite) TestFiles_createDirAppearsInTree() {
	resp := s.Env.POST(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files",
		map[string]any{
			"path": "subdir",
			"type": "dir",
		},
	)
	kit.RequireStatus(s.T(), resp, 201)
	resp.Body.Close()

	resp = s.Env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files/tree?path=.",
	)
	kit.RequireStatus(s.T(), resp, 200)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)

	s.Assert().Contains(
		nodeNames(nodes),
		"subdir",
	)
}

// TestFiles_renameMovesPath verifies Rename makes old path disappear and new
// path appear in the tree.
func (s *FilesSuite) TestFiles_renameMovesPath() {
	resp := s.Env.PATCH(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files",
		map[string]any{
			"path":    "hello.txt",
			"newPath": "renamed.txt",
		},
	)
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	resp = s.Env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files/tree?path=.",
	)
	kit.RequireStatus(s.T(), resp, 200)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)

	names := nodeNames(nodes)
	s.Assert().Contains(
		names,
		"renamed.txt",
	)
	s.Assert().NotContains(
		names,
		"hello.txt",
	)
}

// TestFiles_deleteRemovesFromTree verifies Delete makes the path disappear
// from the next Tree call.
func (s *FilesSuite) TestFiles_deleteRemovesFromTree() {
	resp := s.Env.DELETE(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files?path=hello.txt",
	)
	kit.RequireStatus(s.T(), resp, 200)
	resp.Body.Close()

	resp = s.Env.GET(
		s.T(),
		"/v0/workspaces/"+s.wsID+"/files/tree?path=.",
	)
	kit.RequireStatus(s.T(), resp, 200)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)

	s.Assert().NotContains(
		nodeNames(nodes),
		"hello.txt",
	)
}

// TestFiles_hubBroadcastReachesFilesWatcher verifies that a FileChangeEvent
// pushed directly to the hub is received by a connected files WS subscriber.
// (The OS watcher feeds this path in production; here we inject the event
// directly so the test is deterministic without a running file watcher.)
func (s *FilesSuite) TestFiles_hubBroadcastReachesFilesWatcher() {
	t := s.T()

	watcher := s.Env.DialFiles(
		t,
		"?wsId="+s.wsID,
	)

	s.Env.PushFile(kit.FileEvent{
		WsID: s.wsID,
	})

	msg := watcher.ReadUntil(
		t,
		wsTimeout,
		func(m map[string]any) bool {
			wsID, _ := m["wsId"].(string)
			return wsID == s.wsID
		},
	)
	s.Assert().Equal(
		s.wsID,
		msg["wsId"],
	)
}
