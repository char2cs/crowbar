package content_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/content"
)

func newStore(t *testing.T) *content.Store {
	t.Helper()
	s, err := content.New(filepath.Join(t.TempDir(), "content"))
	require.NoError(t, err)
	return s
}

func TestNew_RefusesAnEmptyRoot(t *testing.T) {
	_, err := content.New("")

	assert.Error(t, err)
}

func TestPut_RoundTripsAPayload(t *testing.T) {
	s := newStore(t)

	ref, err := s.Put([]byte("tool output"))
	require.NoError(t, err)
	require.NotEmpty(t, ref)

	got, err := s.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "tool output", string(got))
}

func TestPut_IsContentAddressedSoIdenticalPayloadsStoreOnce(t *testing.T) {
	s := newStore(t)

	first, err := s.Put([]byte("same bytes"))
	require.NoError(t, err)
	second, err := s.Put([]byte("same bytes"))
	require.NoError(t, err)

	assert.Equal(t, first, second)

	sum := sha256.Sum256([]byte("same bytes"))
	assert.Equal(t, content.RefPrefix+hex.EncodeToString(sum[:]), first)
}

func TestPut_DifferentPayloadsGetDifferentRefs(t *testing.T) {
	s := newStore(t)

	a, err := s.Put([]byte("one"))
	require.NoError(t, err)
	b, err := s.Put([]byte("two"))
	require.NoError(t, err)

	assert.NotEqual(t, a, b)
}

func TestPut_EmptyPayloadHasNoRef(t *testing.T) {
	s := newStore(t)

	ref, err := s.Put(nil)

	require.NoError(t, err)
	assert.Empty(t, ref)
}

func TestPut_TruncatesBeyondTheCeilingAndSaysSo(t *testing.T) {
	s := newStore(t)
	huge := []byte(strings.Repeat("x", content.MaxPayloadBytes+4096))

	ref, err := s.Put(huge)
	require.NoError(t, err)

	got, err := s.Get(ref)
	require.NoError(t, err)
	assert.Less(t, len(got), len(huge))
	assert.Contains(t, string(got), "truncated")
}

func TestGet_MissingOrMalformedRefIsNotFound(t *testing.T) {
	s := newStore(t)

	for _, ref := range []string{
		"",
		"not-a-ref",
		content.RefPrefix + "tooshort",
		content.RefPrefix + strings.Repeat("z", 64),
		content.RefPrefix + strings.Repeat("a", 64),
	} {
		_, err := s.Get(ref)
		assert.ErrorIs(t, err, content.ErrNotFound, "ref %q", ref)
	}
}

func TestGet_RefusesATraversalDisguisedAsARef(t *testing.T) {
	s := newStore(t)

	_, err := s.Get(content.RefPrefix + "../../../../etc/passwd")

	assert.ErrorIs(t, err, content.ErrNotFound)
}

func TestPut_FansOutSoNoSingleDirectoryGrowsUnbounded(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := content.New(root)
	require.NoError(t, err)

	ref, err := s.Put([]byte("payload"))
	require.NoError(t, err)

	digest := strings.TrimPrefix(ref, content.RefPrefix)
	path := filepath.Join(root, digest[:2], digest[2:4], digest)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "payloads are stored two levels deep, keyed by their digest")
}

func TestPut_LeavesNoTemporaryFilesBehind(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := content.New(root)
	require.NoError(t, err)

	_, err = s.Put([]byte("payload"))
	require.NoError(t, err)

	var temps []string
	require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasPrefix(filepath.Base(p), ".tmp-") {
			temps = append(temps, p)
		}
		return nil
	}))
	assert.Empty(t, temps, "the write is temp-then-rename; the temp must not survive")
}

func TestPut_StoresPayloadsReadableOnlyByTheDaemon(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := content.New(root)
	require.NoError(t, err)

	ref, err := s.Put([]byte("private tool output"))
	require.NoError(t, err)

	digest := strings.TrimPrefix(ref, content.RefPrefix)
	info, statErr := os.Stat(filepath.Join(root, digest[:2], digest[2:4], digest))
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestPut_ReportsAFailureToWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := content.New(root)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(root, 0o500))
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	_, err = s.Put([]byte("payload"))

	assert.Error(t, err)
}

func TestNew_ReportsAnUncreatableRoot(t *testing.T) {
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))

	_, err := content.New(filepath.Join(blocker, "content"))

	assert.Error(t, err)
}
