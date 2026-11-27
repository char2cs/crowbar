package adapters

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectPath_AnEmptySegmentFailsClosed(t *testing.T) {
	doc := map[string]any{"items": map[string]any{"name": "x"}}

	got := selectPath([]any{doc}, "items..name")

	assert.Nil(t, got, "an empty path segment must fail closed, not be skipped over")
}

func TestSelectPath_SkipsElementsThatCannotDescendIntoTheKey(t *testing.T) {
	values := []any{"not-an-object", map[string]any{"field": "kept"}, 42}

	got := selectPath(values, "field")

	assert.Equal(t, []any{"kept"}, got)
}

func TestLookupField_NonMapPartwayThroughThePathReturnsNil(t *testing.T) {
	row := map[string]any{"a": "scalar, not nested"}

	got := lookupField(row, "a.b")

	assert.Nil(t, got, "descending into a scalar must fail closed, not panic")
}

func TestLookupField_MissingKeyReturnsNil(t *testing.T) {
	row := map[string]any{"a": map[string]any{"present": true}}

	got := lookupField(row, "a.missing")

	assert.Nil(t, got)
}

func TestLiteralSections_AnUnterminatedSectionIsDroppedNotHalfReturned(t *testing.T) {
	text := "before<start>closed</end>after<start>never closed"

	got := literalSections(text, "<start>", "</end>")

	assert.Equal(t, []string{"closed"}, got)
}
