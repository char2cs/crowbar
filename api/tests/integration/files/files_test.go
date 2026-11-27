//go:build integration

package files_test

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

type FilesSuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
	wsID     string
	chatID   string
}

func (s *FilesSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	repoPath := kit.InitRepoWithFile(s.T(), "hello.txt", "hello world\n")
	s.imported = s.Env.ImportRepo(s.T(), "files", repoPath)
	s.wsID, s.chatID = s.Env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/test-files", "",
	)
}

func (s *FilesSuite) base() string {
	return "/v0/chats/" + s.chatID
}

const documentsContent = "/v0/documents/content"

func (s *FilesSuite) doc(path string) string {
	return documentsContent + "?" + url.Values{"workspace": {s.wsID}, "path": {path}}.Encode()
}

func (s *FilesSuite) save(path string, body map[string]any) *http.Response {
	body["workspaceId"], body["path"] = s.wsID, path
	return s.Env.PUT(s.T(), documentsContent, body)
}

func TestFilesSuite(
	t *testing.T,
) {
	suite.Run(t, new(FilesSuite))
}

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

func (s *FilesSuite) TestFiles_treeReturnsRootEntries() {
	resp := s.Env.GET(s.T(), s.base()+"/files/tree?path=.")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var nodes []map[string]any
	kit.DecodeEnvData(s.T(), resp, &nodes)
	s.Assert().Contains(nodeNames(nodes), "hello.txt")
}

func (s *FilesSuite) TestFiles_readContentReturnsFileBytes() {
	resp := s.Env.GET(s.T(), s.doc("hello.txt")) //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Assert().Equal("hello world\n", content["content"])
}

func (s *FilesSuite) TestFiles_writeContentMutatesAndResyncs() {
	resp := s.save("hello.txt", map[string]any{"content": "updated content\n"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.doc("hello.txt")) //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Assert().Equal("updated content\n", content["content"])
}

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

func (s *FilesSuite) TestRegression_Files_BinaryReadIsByteFaithfulThroughJSON() {
	original := bytes.Repeat([]byte("x"), 8192)
	original = append(original, 0xE9)
	original = append(original, []byte("trailer")...)
	encoded := base64.StdEncoding.EncodeToString(original)

	resp := s.save("assets/late.bin", map[string]any{"content": encoded, "encoding": "base64"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.doc("assets/late.bin")) //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Require().Equal("base64", content["encoding"], "invalid-UTF-8 file must read back as base64")
	gotBytes, err := base64.StdEncoding.DecodeString(content["content"].(string))
	s.Require().NoError(err)
	s.Assert().Equal(original, gotBytes, "round-trip through the JSON layer must be byte-identical")
}

func (s *FilesSuite) TestRegression_Files_RenameRefusesToClobber() {
	writeText := func(path, body string) {
		resp := s.save(path, map[string]any{"content": body})
		kit.RequireStatus(s.T(), resp, http.StatusOK)
		resp.Body.Close()
	}
	writeText("src.txt", "SOURCE")
	writeText("dst.txt", "PRECIOUS")

	resp := s.Env.PATCH(s.T(), s.base()+"/files", map[string]any{"path": "src.txt", "newPath": "dst.txt"})
	kit.RequireStatus(s.T(), resp, http.StatusConflict)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.doc("dst.txt")) //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Assert().Equal("PRECIOUS", content["content"], "the clobbered file's content must be preserved")
}

func (s *FilesSuite) TestRegression_Files_CopyBinaryByteFaithful() {
	original := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0xFF, 0xFE, 0x00, 0x01}
	encoded := base64.StdEncoding.EncodeToString(original)

	resp := s.save("logo.png", map[string]any{"content": encoded, "encoding": "base64"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.POST(s.T(), s.base()+"/files/copy", map[string]any{
		"sourcePath": "logo.png", "destPath": "logo copy.png",
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.doc("logo copy.png")) //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Require().Equal("base64", content["encoding"])
	gotBytes, err := base64.StdEncoding.DecodeString(content["content"].(string))
	s.Require().NoError(err)
	s.Assert().Equal(original, gotBytes, "copied binary must be byte-identical")

	resp = s.Env.POST(s.T(), s.base()+"/files/copy", map[string]any{
		"sourcePath": "logo.png", "destPath": "logo copy.png",
	})
	kit.RequireStatus(s.T(), resp, http.StatusConflict)
	resp.Body.Close()
}

func (s *FilesSuite) TestRegression_OpenFileOutsideWorkspace() {
	dir, err := os.MkdirTemp("", "host-doc")
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = os.RemoveAll(dir) })
	abs := filepath.Join(dir, "outside.md")
	s.Require().NoError(os.WriteFile(abs, []byte("outside\n"), 0o600))

	resp := s.Env.GET(s.T(), documentsContent+"?path="+url.QueryEscape(abs)) //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var doc map[string]any
	kit.DecodeEnvData(s.T(), resp, &doc)
	s.Equal("host", doc["authority"])
	s.Equal(safepath.Canonical(abs), doc["path"])
	s.Equal("outside\n", doc["content"])

	resp = s.Env.PUT(s.T(), documentsContent, map[string]any{"path": abs, "content": "edited\n"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	s.Require().NoError(resp.Body.Close())
	got, err := os.ReadFile(abs)
	s.Require().NoError(err)
	s.Equal("edited\n", string(got))
}

func (s *FilesSuite) TestRegression_AbsolutePathInsideWorktreeIsWorkspaceDocument() {
	wt := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.wsID)
	dir, err := os.MkdirTemp("", "wt-link")
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = os.RemoveAll(dir) })
	link := filepath.Join(dir, "wt-link")
	s.Require().NoError(os.Symlink(wt, link))

	for _, abs := range []string{filepath.Join(wt, "hello.txt"), filepath.Join(link, "hello.txt")} {
		resp := s.Env.GET(s.T(), documentsContent+"?path="+url.QueryEscape(abs)) //nolint:bodyclose // DecodeEnvData closes the body
		kit.RequireStatus(s.T(), resp, http.StatusOK)
		var doc map[string]any
		kit.DecodeEnvData(s.T(), resp, &doc)
		s.Equal("workspace", doc["authority"], abs)
		s.Equal(s.wsID, doc["workspaceId"], abs)
		s.Equal("hello.txt", doc["path"], abs)
	}
}
