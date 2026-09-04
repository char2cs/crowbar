package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestResolveOriginalName_KeepsAGivenFilename proves a client-supplied
// filename passes through verbatim.
func TestResolveOriginalName_KeepsAGivenFilename(t *testing.T) {
	assert.Equal(t, "photo.png", resolveOriginalName("photo.png", []byte("data")))
}

// TestResolveOriginalName_SynthesizesFromContentTypeWhenBlank proves the
// backstop branch net/http's own multipart parser can never itself trigger
// (a FILE part with an explicitly empty filename is reclassified as a form
// VALUE before this code ever runs — see attachments_test.go's own note) —
// exercised directly here so the defensive branch still has real coverage.
func TestResolveOriginalName_SynthesizesFromContentTypeWhenBlank(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	assert.Regexp(t, `^pasted-image-.+\.png$`, resolveOriginalName("", png))
}
