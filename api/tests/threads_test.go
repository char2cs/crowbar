//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type threadDTO struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	FilePath  string `json:"filePath"`
	Line      int    `json:"line"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Side      string `json:"side"`
	Body      string `json:"body"`
	Author    string `json:"author"`
	IsAgent   bool   `json:"isAgent"`
}

// TestStream_WsScopeCarriesReviewThreads: a client holding the workspace's ws:
// scope gets its threads as the snapshot, hears a new comment as a
// review_thread entity frame, and a deleted thread as gone.
func TestStream_WsScopeCarriesReviewThreads(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := threadsBase(imported)
	scope := "ws:" + imported.workspaceID
	var first threadDTO
	h.post(base+"/threads", map[string]any{
		"filePath": "README.md", "line": 1, "side": "right", "body": "before the hold",
	}, http.StatusCreated, &first)
	isThread := func(id string) func(stream.Frame) bool {
		return func(f stream.Frame) bool {
			return f.T == stream.FrameEntity && f.Scope == scope && f.Kind == string(stream.KindReviewThread) && f.ID == id
		}
	}

	c := dialStream(t, h)
	c.Sub(t, scope, 0)
	c.Next(t, isThread(first.ID))
	var second threadDTO
	h.post(base+"/threads", map[string]any{
		"filePath": "README.md", "line": 2, "side": "right", "body": "after the hold",
	}, http.StatusCreated, &second)
	f := c.Next(t, isThread(second.ID))
	var got threadDTO
	require.NoError(t, json.Unmarshal(f.Data, &got))
	assert.Equal(t, "after the hold", got.Body)

	h.del(base+"/threads/"+first.ID, nil, http.StatusOK, nil)
	gone := c.Next(t, isThread(first.ID))
	assert.True(t, gone.Gone)
}

// TestThreads_RangeAnchor proves a thread can be anchored to a multi-line range.
func TestThreads_RangeAnchor(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := threadsBase(imported)

	var got threadDTO
	h.post(base+"/threads", map[string]any{
		"filePath":  "README.md",
		"line":      44,
		"startLine": 42,
		"endLine":   44,
		"side":      "right",
		"body":      "guard this range",
	}, http.StatusCreated, &got)

	assert.Equal(t, 42, got.StartLine)
	assert.Equal(t, 44, got.EndLine)
	assert.Equal(t, 44, got.Line)
}

// TestThreads_SingleLineDefaultsRange proves that omitting startLine/endLine
// causes the aggregate to default both to line, enforcing the single-line
// invariant for all callers (not just the HTTP handler).
func TestThreads_SingleLineDefaultsRange(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := threadsBase(imported)

	var got threadDTO
	h.post(base+"/threads", map[string]any{
		"filePath": "README.md",
		"line":     7,
		"side":     "right",
		"body":     "single-line invariant",
	}, http.StatusCreated, &got)

	assert.Equal(t, 7, got.Line)
	assert.Equal(t, 7, got.StartLine)
	assert.Equal(t, 7, got.EndLine)
}

type threadReplyDTO struct {
	ID      string `json:"id"`
	Body    string `json:"body"`
	Author  string `json:"author"`
	IsAgent bool   `json:"isAgent"`
}

type threadWithReplies struct {
	threadDTO
	Replies []threadReplyDTO `json:"replies"`
}

// TestThreads_AuthorAndIsAgent proves human vs agent authorship round-trips on
// open and reply.
func TestThreads_AuthorAndIsAgent(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := threadsBase(imported)

	var opened threadWithReplies
	h.post(base+"/threads", map[string]any{
		"filePath": "README.md", "line": 10, "side": "right",
		"author": "mateourru", "isAgent": false, "body": "@claude take a look",
	}, http.StatusCreated, &opened)
	assert.Equal(t, "mateourru", opened.Author)
	assert.False(t, opened.IsAgent)

	var replied threadWithReplies
	h.post(base+"/threads/"+opened.ID+"/replies", map[string]any{
		"author": "claude", "isAgent": true, "body": "on it",
	}, http.StatusOK, &replied)

	require.Len(t, replied.Replies, 1)
	assert.Equal(t, "claude", replied.Replies[0].Author)
	assert.True(t, replied.Replies[0].IsAgent, "agent reply must carry isAgent")
}

// TestRegression_GeneralComment_OpensRepliesResolvesAndLists proves a thread
// with no anchor (the Conversation tab's general comment) works end to end.
func TestRegression_GeneralComment_OpensRepliesResolvesAndLists(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := threadsBase(imported)

	var opened threadWithReplies
	h.post(base+"/threads", map[string]any{
		"filePath": "", "line": 0, "startLine": 0, "endLine": 0,
		"side": "new", "body": "overall this needs a changelog entry",
	}, http.StatusCreated, &opened)
	assert.Empty(t, opened.FilePath)
	assert.Zero(t, opened.Line)
	assert.Zero(t, opened.StartLine)
	assert.Zero(t, opened.EndLine)

	var replied threadWithReplies
	h.post(base+"/threads/"+opened.ID+"/replies", map[string]any{
		"body": "added",
	}, http.StatusOK, &replied)
	require.Len(t, replied.Replies, 1)

	var resolved struct {
		Resolved bool `json:"resolved"`
	}
	h.patch(base+"/threads/"+opened.ID, map[string]any{"isResolved": true}, &resolved)
	assert.True(t, resolved.Resolved)

	var listed []threadDTO
	h.get(base+"/threads", &listed)
	require.Len(t, listed, 1)
	assert.Equal(t, opened.ID, listed[0].ID)
	assert.Empty(t, listed[0].FilePath)
}

// TestRegression_HalfAnchoredThread_IsRejected proves a partial anchor is
// refused rather than stored as a thread no diff line can show.
func TestRegression_HalfAnchoredThread_IsRejected(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := threadsBase(imported)

	for _, body := range []map[string]any{
		{"filePath": "README.md", "side": "right", "body": "path without a line"},
		{"line": 4, "side": "right", "body": "line without a path"},
		{"startLine": 2, "endLine": 3, "side": "right", "body": "range without a path"},
		{"filePath": "README.md", "line": 3, "startLine": 5, "endLine": 3, "side": "right", "body": "end before start"},
		{"filePath": "README.md", "line": -1, "side": "right", "body": "negative line"},
	} {
		h.postError(base+"/threads", body, http.StatusUnprocessableEntity)
	}
}
