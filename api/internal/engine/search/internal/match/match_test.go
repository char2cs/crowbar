package match_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/search/internal/match"
)

func TestBuildPattern_FixedString(t *testing.T) {
	re, err := match.BuildPattern("hello.world", true, false, false)
	require.NoError(t, err)
	assert.True(t, re.MatchString("hello.world"))
	assert.False(t, re.MatchString("helloXworld"))
}

func TestBuildPattern_CaseInsensitive(t *testing.T) {
	re, err := match.BuildPattern("hello", false, false, false)
	require.NoError(t, err)
	assert.True(t, re.MatchString("HELLO"))
	assert.True(t, re.MatchString("Hello"))
}

func TestBuildPattern_WholeWord(t *testing.T) {
	re, err := match.BuildPattern("hello", true, true, false)
	require.NoError(t, err)
	assert.True(t, re.MatchString("say hello world"))
	assert.False(t, re.MatchString("hellofoo"))
}

func TestBuildPattern_Regex(t *testing.T) {
	re, err := match.BuildPattern(`foo\d+`, true, false, true)
	require.NoError(t, err)
	assert.True(t, re.MatchString("foo123"))
	assert.False(t, re.MatchString("foobar"))
}

func TestBuildPattern_EmptyQuery(t *testing.T) {
	_, err := match.BuildPattern("", true, false, false)
	require.Error(t, err)
}

func TestBuildPattern_InvalidRegex(t *testing.T) {
	_, err := match.BuildPattern("[invalid", true, false, true)
	require.Error(t, err)
}

func TestFindMatches_None(t *testing.T) {
	re, err := match.BuildPattern("xyz", true, false, false)
	require.NoError(t, err)
	spans := match.FindMatches(re, "hello world")
	assert.Nil(t, spans)
}

func TestFindMatches_Single(t *testing.T) {
	re, err := match.BuildPattern("world", true, false, false)
	require.NoError(t, err)
	spans := match.FindMatches(re, "hello world")
	require.Len(t, spans, 1)
	assert.Equal(t, 6, spans[0].Start)
	assert.Equal(t, 11, spans[0].End)
}

func TestFindMatches_Multiple(t *testing.T) {
	re, err := match.BuildPattern("ab", true, false, false)
	require.NoError(t, err)
	spans := match.FindMatches(re, "ab cd ab ef ab")
	require.Len(t, spans, 3)
}

func TestFindMatches_UTF16Surrogate(t *testing.T) {
	// "😀" is U+1F600, encodes as surrogate pair in UTF-16 (2 code units).
	// "ab" starts at byte offset 4 (after the emoji + space).
	re, err := match.BuildPattern("ab", true, false, false)
	require.NoError(t, err)
	// emoji (4 bytes) + space (1 byte) + "ab"
	spans := match.FindMatches(re, "😀 ab")
	require.Len(t, spans, 1)
	// emoji = 2 UTF-16 units, space = 1, so "ab" starts at col 3
	assert.Equal(t, 3, spans[0].Start)
	assert.Equal(t, 5, spans[0].End)
}

func TestIsBinary_WithNull(t *testing.T) {
	assert.True(t, match.IsBinary([]byte{'a', 'b', 0x00, 'c'}))
}

func TestIsBinary_WithoutNull(t *testing.T) {
	assert.False(t, match.IsBinary([]byte("hello world")))
}

func TestIsBinary_Empty(t *testing.T) {
	assert.False(t, match.IsBinary([]byte{}))
}
