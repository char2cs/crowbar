//go:build integration

package files_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

type HomeFilesSuite struct {
	kit.IntegrationSuite
	projectID   string
	projectPath string
	homeID      string
}

func (s *HomeFilesSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.projectPath = s.T().TempDir()
	s.projectID = s.Env.RegisterProject(s.T(), "home-files", s.projectPath)
	resp := s.Env.GET(s.T(), s.base()) //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var home map[string]any
	kit.DecodeEnvData(s.T(), resp, &home)
	s.homeID, _ = home["id"].(string)
	s.Require().NotEmpty(s.homeID, "the home endpoint must name its workspace")
}

func TestHomeFilesSuite(
	t *testing.T,
) {
	suite.Run(t, new(HomeFilesSuite))
}

func (s *HomeFilesSuite) base() string {
	return "/v0/projects/" + s.projectID + "/home"
}

func (s *HomeFilesSuite) TestRegression_HomeSaveFileContent_NonGitProjectDir() {
	s.Require().NoError(
		os.WriteFile(filepath.Join(s.projectPath, "NOTES.md"), []byte("# before\n"), 0o600),
	)

	resp := s.Env.PUT(s.T(), documentsContent, map[string]any{
		"workspaceId": s.homeID,
		"path":        "NOTES.md",
		"content":     "# after\n",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), documentsContent+"?"+url.Values{"workspace": {s.homeID}, "path": {"NOTES.md"}}.Encode()) //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Assert().Equal("# after\n", content["content"])
}

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
