package v0

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newMockRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/branch-review/:wsId/diff", BranchReviewDiff)
	r.GET("/branch-review/:wsId/chats", BranchReviewChats)
	r.GET("/branch-review/:wsId/threads", BranchReviewThreads)
	r.GET("/branch-review/:wsId/description", BranchReviewDescription)
	r.GET("/fs/file", FsFile)
	r.GET("/markdown-chat/:wsId/:stepId", MarkdownChat)
	return r
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestBranchReviewDiffServesAMultiFileDiff(t *testing.T) {
	w := get(newMockRouter(), "/branch-review/ws3/diff")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var diff map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &diff); err != nil {
		t.Fatal(err)
	}
	if diff["totalFiles"].(float64) != 1 {
		t.Fatalf("totalFiles = %v, want 1", diff["totalFiles"])
	}
	if _, ok := diff["files"].([]any); !ok {
		t.Fatalf("files is not an array: %T", diff["files"])
	}
}

func TestBranchReviewChatsServesAnArray(t *testing.T) {
	w := get(newMockRouter(), "/branch-review/ws3/chats")
	var chats []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &chats); err != nil {
		t.Fatal(err)
	}
	if len(chats) == 0 {
		t.Fatal("expected at least one chat")
	}
	if _, ok := chats[0]["isActive"].(bool); !ok {
		t.Fatal("chat missing isActive")
	}
}

func TestBranchReviewThreadsAndDescriptionAreValid(t *testing.T) {
	r := newMockRouter()

	threads := get(r, "/branch-review/ws3/threads")
	if threads.Body.String() != "[]" {
		t.Fatalf("threads = %q, want []", threads.Body.String())
	}

	desc := get(r, "/branch-review/ws3/description")
	var s string
	if err := json.Unmarshal(desc.Body.Bytes(), &s); err != nil {
		t.Fatalf("description is not a JSON string: %v", err)
	}
}

func TestFsFileEchoesThePathAsAString(t *testing.T) {
	w := get(newMockRouter(), "/fs/file?path=web/vite.config.ts")
	var s string
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("file content is not a JSON string: %v", err)
	}
	if s == "" {
		t.Fatal("expected non-empty file content")
	}
}

func TestMarkdownChatServesTurns(t *testing.T) {
	w := get(newMockRouter(), "/markdown-chat/ws3/step-1")
	var turns []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &turns); err != nil {
		t.Fatal(err)
	}
	if len(turns) < 2 {
		t.Fatalf("turns = %d, want >= 2", len(turns))
	}
	if _, ok := turns[0]["widgets"].([]any); !ok {
		t.Fatal("turn missing widgets array")
	}
}
