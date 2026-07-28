//go:build integration

package files_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// HomeFilesSuite exercises the project-level home workspace's file surface
// (/v0/projects/:projectId/home/files/...). The home workspace is rooted at the
// PROJECT directory, which — unlike every repo workspace — is deliberately NOT a
// git repository (a project is a folder that CONTAINS repos). Every test here
// therefore registers a project on a plain, non-git temp dir.
type HomeFilesSuite struct {
	kit.IntegrationSuite
	projectID   string
	projectPath string
}

func (s *HomeFilesSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	// A plain directory with no .git anywhere above it: `git rev-parse` walks
	// upward, so a tmpdir under a git checkout would silently make this a git
	// workspace and mask the bug.
	s.projectPath = s.T().TempDir()
	s.projectID = s.Env.RegisterProject(s.T(), "home-files", s.projectPath)
}

// TestHomeFilesSuite is the testify suite entry point for HomeFilesSuite.
func TestHomeFilesSuite(
	t *testing.T,
) {
	suite.Run(t, new(HomeFilesSuite))
}

func (s *HomeFilesSuite) base() string {
	return "/v0/projects/" + s.projectID + "/home"
}

// TestRegression_HomeSaveFileContent_NonGitProjectDir pins that saving a file in
// the project home workspace SUCCEEDS. The write path resyncs the workspace's
// working-tree summary after every mutation, and that summary shells out to git;
// in the non-git project directory those git invocations exit non-zero, which
// used to fail the whole request with a 500 — the file landed on disk but the
// editor reported "Failed to save file" and kept the buffer dirty.
func (s *HomeFilesSuite) TestRegression_HomeSaveFileContent_NonGitProjectDir() {
	s.Require().NoError(
		os.WriteFile(filepath.Join(s.projectPath, "NOTES.md"), []byte("# before\n"), 0o600),
	)

	resp := s.Env.PUT(s.T(), s.base()+"/files/content", map[string]any{
		"path":    "NOTES.md",
		"content": "# after\n",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/files/content?path=NOTES.md")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Assert().Equal("# after\n", content["content"])
}

// TestRegression_HomeCreateFile_NonGitProjectDir pins the same resync tolerance
// for creation — the home explorer's "New File" runs through the identical
// write-then-resync path.
func (s *HomeFilesSuite) TestRegression_HomeCreateFile_NonGitProjectDir() {
	resp := s.Env.POST(s.T(), s.base()+"/files", map[string]any{
		"path": "fresh.md",
		"type": "file",
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/files/tree?path=.")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)
	s.Assert().Contains(nodeNames(nodes), "fresh.md")
}

// TestRegression_HomeRenameAndDelete_NonGitProjectDir pins the remaining two
// home mutations against the same non-git resync.
func (s *HomeFilesSuite) TestRegression_HomeRenameAndDelete_NonGitProjectDir() {
	s.Require().NoError(
		os.WriteFile(filepath.Join(s.projectPath, "old.md"), []byte("x\n"), 0o600),
	)

	resp := s.Env.PATCH(s.T(), s.base()+"/files", map[string]any{
		"path":    "old.md",
		"newPath": "new.md",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.DELETEJ(s.T(), s.base()+"/files", map[string]any{"path": "new.md"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	_, err := os.Stat(filepath.Join(s.projectPath, "new.md"))
	s.Assert().True(os.IsNotExist(err), "delete must remove the file from the project dir")
}
