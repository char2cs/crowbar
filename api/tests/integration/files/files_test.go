//go:build integration

package files_test

import (
	"bytes"
	"encoding/base64"
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
	chatID   string
}

func (s *FilesSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	repoPath := kit.InitRepoWithFile(s.T(), "hello.txt", "hello world\n")
	s.imported = s.Env.ImportRepo(s.T(), "files", repoPath)
	// A writable child forked from main inherits the committed hello.txt.
	s.wsID, s.chatID = s.Env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/test-files", "",
	)
}

// base returns the chat-scoped route prefix for the writable child.
func (s *FilesSuite) base() string {
	return "/v0/chats/" + s.chatID
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

// TestRegression_Files_BinaryReadIsByteFaithfulThroughJSON pins the
// silent-corruption fix end to end, through the real HTTP + gin JSON layer. A
// file whose first 8 KiB is clean UTF-8 but which holds an invalid byte later
// (0xE9) was classified as text and returned as a JSON string; encoding/json then
// rewrote the invalid byte to U+FFFD, so the client received corrupted content
// and a save persisted it. The read must now come back base64-encoded and decode
// to the EXACT original bytes.
func (s *FilesSuite) TestRegression_Files_BinaryReadIsByteFaithfulThroughJSON() {
	original := bytes.Repeat([]byte("x"), 8192)
	original = append(original, 0xE9) // invalid standalone UTF-8, past the old 8 KiB window
	original = append(original, []byte("trailer")...)
	encoded := base64.StdEncoding.EncodeToString(original)

	// Write byte-faithfully via the base64 encoding.
	resp := s.Env.PUT(s.T(), s.base()+"/files/content", map[string]any{
		"path":     "assets/late.bin",
		"content":  encoded,
		"encoding": "base64",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	// Read it back: must be base64, and decode to the exact original bytes.
	resp = s.Env.GET(s.T(), s.base()+"/files/content?path=assets/late.bin")
	kit.RequireStatus(s.T(), resp, http.StatusOK)

	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Require().Equal("base64", content["encoding"], "invalid-UTF-8 file must read back as base64")
	gotBytes, err := base64.StdEncoding.DecodeString(content["content"].(string))
	s.Require().NoError(err)
	s.Assert().Equal(original, gotBytes, "round-trip through the JSON layer must be byte-identical")
}

// TestRegression_Files_RenameRefusesToClobber pins the rename/move data-loss fix:
// renaming onto an existing path must 409 rather than silently destroy the
// occupant. This is the same daemon verb the FE uses for both inline rename and
// drag-drop move, so the guard protects every collision path at once.
func (s *FilesSuite) TestRegression_Files_RenameRefusesToClobber() {
	writeText := func(path, body string) {
		resp := s.Env.PUT(s.T(), s.base()+"/files/content", map[string]any{"path": path, "content": body})
		kit.RequireStatus(s.T(), resp, http.StatusOK)
		resp.Body.Close()
	}
	writeText("src.txt", "SOURCE")
	writeText("dst.txt", "PRECIOUS")

	resp := s.Env.PATCH(s.T(), s.base()+"/files", map[string]any{"path": "src.txt", "newPath": "dst.txt"})
	kit.RequireStatus(s.T(), resp, http.StatusConflict)
	resp.Body.Close()

	// The occupant survived untouched.
	resp = s.Env.GET(s.T(), s.base()+"/files/content?path=dst.txt")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Assert().Equal("PRECIOUS", content["content"], "the clobbered file's content must be preserved")
}

// TestRegression_Files_CopyBinaryByteFaithful exercises the copy verb over HTTP:
// a binary file copies byte-identically (the reason the server-side io.Copy verb
// exists — a client read/write composition would corrupt it), and a second copy
// onto the now-existing destination is a 409 rather than a silent clobber.
func (s *FilesSuite) TestRegression_Files_CopyBinaryByteFaithful() {
	original := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0xFF, 0xFE, 0x00, 0x01}
	encoded := base64.StdEncoding.EncodeToString(original)

	resp := s.Env.PUT(s.T(), s.base()+"/files/content", map[string]any{
		"path": "logo.png", "content": encoded, "encoding": "base64",
	})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	resp = s.Env.POST(s.T(), s.base()+"/files/copy", map[string]any{
		"sourcePath": "logo.png", "destPath": "logo copy.png",
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	resp.Body.Close()

	resp = s.Env.GET(s.T(), s.base()+"/files/content?path=logo%20copy.png")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var content map[string]any
	kit.DecodeEnvData(s.T(), resp, &content)
	s.Require().Equal("base64", content["encoding"])
	gotBytes, err := base64.StdEncoding.DecodeString(content["content"].(string))
	s.Require().NoError(err)
	s.Assert().Equal(original, gotBytes, "copied binary must be byte-identical")

	// Copying again onto the existing destination must 409, never clobber.
	resp = s.Env.POST(s.T(), s.base()+"/files/copy", map[string]any{
		"sourcePath": "logo.png", "destPath": "logo copy.png",
	})
	kit.RequireStatus(s.T(), resp, http.StatusConflict)
	resp.Body.Close()
}

// TestFiles_hubBroadcastReachesFilesWatcher verifies that a FileChangeEvent
// pushed directly to the hub is received by a connected files WS subscriber on
// the chat-scoped, co-located .../files/ws route.
func (s *FilesSuite) TestFiles_hubBroadcastReachesFilesWatcher() {
	t := s.T()

	watcher := s.Env.DialFiles(t, s.chatID)
	s.Env.PushFile(kit.FileEvent{WsID: s.wsID})

	msg := watcher.ReadUntil(t, wsTimeout, func(m map[string]any) bool {
		wsID, _ := m["wsId"].(string)
		return wsID == s.wsID
	})
	s.Assert().Equal(s.wsID, msg["wsId"])
}
