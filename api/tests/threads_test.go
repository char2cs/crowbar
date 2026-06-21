//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

type threadDTO struct {
	ID        string `json:"id"`
	FilePath  string `json:"filePath"`
	Line      int    `json:"line"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Side      string `json:"side"`
	Body      string `json:"body"`
	Author    string `json:"author"`
	IsAgent   bool   `json:"isAgent"`
}

// TestThreads_RangeAnchor proves a thread can be anchored to a multi-line range.
func TestThreads_RangeAnchor(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

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
	base := wsBase(imported)

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
