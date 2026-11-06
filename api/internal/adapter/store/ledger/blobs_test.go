package ledger_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func newBlobs(t *testing.T) *ledgerstore.Blobs {
	t.Helper()
	s, err := ledgerstore.OpenBlobs(filepath.Join(t.TempDir(), "content"))
	require.NoError(t, err)
	return s
}

func TestOpenBlobs_RefusesAnEmptyRoot(t *testing.T) {
	_, err := ledgerstore.OpenBlobs("")

	assert.Error(t, err)
}

func TestBlobsPut_RoundTripsAPayload(t *testing.T) {
	s := newBlobs(t)

	ref, err := s.Put([]byte("tool output"))
	require.NoError(t, err)
	require.NotEmpty(t, ref)

	got, err := s.Get(ref)
	require.NoError(t, err)
	assert.Equal(t, "tool output", string(got))
}

func TestBlobsPut_IsContentAddressedSoIdenticalPayloadsStoreOnce(t *testing.T) {
	s := newBlobs(t)

	first, err := s.Put([]byte("same bytes"))
	require.NoError(t, err)
	second, err := s.Put([]byte("same bytes"))
	require.NoError(t, err)

	assert.Equal(t, first, second)

	sum := sha256.Sum256([]byte("same bytes"))
	assert.Equal(t, ledgerstore.RefPrefix+hex.EncodeToString(sum[:]), first)
}

func TestBlobsPut_DifferentPayloadsGetDifferentRefs(t *testing.T) {
	s := newBlobs(t)

	a, err := s.Put([]byte("one"))
	require.NoError(t, err)
	b, err := s.Put([]byte("two"))
	require.NoError(t, err)

	assert.NotEqual(t, a, b)
}

func TestBlobsPut_EmptyPayloadHasNoRef(t *testing.T) {
	s := newBlobs(t)

	ref, err := s.Put(nil)

	require.NoError(t, err)
	assert.Empty(t, ref)
}

func TestBlobsPut_TruncatesBeyondTheCeilingAndSaysSo(t *testing.T) {
	s := newBlobs(t)
	huge := []byte(strings.Repeat("x", ledgerstore.MaxPayloadBytes+4096))

	ref, err := s.Put(huge)
	require.NoError(t, err)

	got, err := s.Get(ref)
	require.NoError(t, err)
	assert.Less(t, len(got), len(huge))
	assert.Contains(t, string(got), "truncated")
}

func inUse(used bool) func(string) (bool, error) {
	return func(string) (bool, error) { return used, nil }
}

func TestBlobsCollect_RemovesABlobNoRowNames(t *testing.T) {
	s := newBlobs(t)
	ref, err := s.Put([]byte("tool output"))
	require.NoError(t, err)

	require.NoError(t, s.Collect(ref, inUse(true)))
	_, err = s.Get(ref)
	require.NoError(t, err, "a blob a row names is kept")

	require.NoError(t, s.Collect(ref, inUse(false)))
	_, err = s.Get(ref)
	require.ErrorIs(t, err, ledgerstore.ErrBlobNotFound)
	assert.NoError(t, s.Collect(ref, inUse(false)), "a missing blob is a no-op")
}

func TestBlobsCollect_LeavesNoEmptyShardBehind(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := ledgerstore.OpenBlobs(root)
	require.NoError(t, err)
	gone, err := s.Put([]byte("collected"))
	require.NoError(t, err)
	kept, err := s.Put([]byte("kept"))
	require.NoError(t, err)

	require.NoError(t, s.Collect(gone, inUse(false)))

	var dirs []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return err
	}))
	assert.Len(t, dirs, 2, "only the kept blob's two shard levels remain: %v", dirs)
	_, err = s.Get(kept)
	require.NoError(t, err)
	require.NoError(t, s.Collect(kept, inUse(false)))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "the root stays, empty")
}

func TestBlobsCollect_IsANoOpOnAnEmptyOrMalformedRef(t *testing.T) {
	s := newBlobs(t)
	assert.NoError(t, s.Collect("", inUse(false)))
	assert.NoError(t, s.Collect("not-a-ref", inUse(false)))
}

func TestBlobsCollect_ReportsAFailedCheckAndKeepsTheBlob(t *testing.T) {
	s := newBlobs(t)
	ref, err := s.Put([]byte("tool output"))
	require.NoError(t, err)

	boom := errors.New("boom")
	require.ErrorIs(t, s.Collect(ref, func(string) (bool, error) { return false, boom }), boom)
	_, err = s.Get(ref)
	assert.NoError(t, err)
}

func TestBlobsGet_MissingOrMalformedRefIsNotFound(t *testing.T) {
	s := newBlobs(t)

	for _, ref := range []string{
		"",
		"not-a-ref",
		ledgerstore.RefPrefix + "tooshort",
		ledgerstore.RefPrefix + strings.Repeat("z", 64),
		ledgerstore.RefPrefix + strings.Repeat("a", 64),
	} {
		_, err := s.Get(ref)
		assert.ErrorIs(t, err, ledgerstore.ErrBlobNotFound, "ref %q", ref)
	}
}

func TestBlobsGet_RefusesATraversalDisguisedAsARef(t *testing.T) {
	s := newBlobs(t)

	_, err := s.Get(ledgerstore.RefPrefix + "../../../../etc/passwd")

	assert.ErrorIs(t, err, ledgerstore.ErrBlobNotFound)
}

func TestBlobsPut_FansOutSoNoSingleDirectoryGrowsUnbounded(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := ledgerstore.OpenBlobs(root)
	require.NoError(t, err)

	ref, err := s.Put([]byte("payload"))
	require.NoError(t, err)

	digest := strings.TrimPrefix(ref, ledgerstore.RefPrefix)
	path := filepath.Join(root, digest[:2], digest[2:4], digest)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "payloads are stored two levels deep, keyed by their digest")
}

func TestBlobsPut_LeavesNoTemporaryFilesBehind(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := ledgerstore.OpenBlobs(root)
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

func TestBlobsPut_StoresPayloadsReadableOnlyByTheDaemon(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := ledgerstore.OpenBlobs(root)
	require.NoError(t, err)

	ref, err := s.Put([]byte("private tool output"))
	require.NoError(t, err)

	digest := strings.TrimPrefix(ref, ledgerstore.RefPrefix)
	info, statErr := os.Stat(filepath.Join(root, digest[:2], digest[2:4], digest))
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestBlobsPut_ReportsAFailureToWrite(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	root := filepath.Join(t.TempDir(), "content")
	s, err := ledgerstore.OpenBlobs(root)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(root, 0o500))
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	_, err = s.Put([]byte("payload"))

	assert.Error(t, err)
}

func TestOpenBlobs_ReportsAnUncreatableRoot(t *testing.T) {
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))

	_, err := ledgerstore.OpenBlobs(filepath.Join(blocker, "content"))

	assert.Error(t, err)
}

// unreadableBlob puts a payload, then swaps its file for a non-empty directory
// so it can neither be read nor deleted.
func unreadableBlob(t *testing.T) (*ledgerstore.Blobs, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "content")
	s, err := ledgerstore.OpenBlobs(root)
	require.NoError(t, err)
	ref, err := s.Put([]byte("tool output"))
	require.NoError(t, err)
	digest := strings.TrimPrefix(ref, ledgerstore.RefPrefix)
	path := filepath.Join(root, digest[:2], digest[2:4], digest)
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.MkdirAll(filepath.Join(path, "child"), 0o700))
	return s, ref
}

func TestBlobsGet_AnUnreadableBlobIsAnErrorNotAMissingOne(t *testing.T) {
	s, ref := unreadableBlob(t)

	_, err := s.Get(ref)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ledgerstore.ErrBlobNotFound)
}

func TestBlobsCollect_ReportsABlobItCannotDelete(t *testing.T) {
	s, ref := unreadableBlob(t)

	assert.Error(t, s.Collect(ref, inUse(false)))
}

func TestBlobsPut_ReportsAFanOutDirectoryItCannotCreate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	s, err := ledgerstore.OpenBlobs(root)
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("payload"))
	require.NoError(t, os.WriteFile(filepath.Join(root, hex.EncodeToString(sum[:])[:2]), nil, 0o600))

	_, err = s.Put([]byte("payload"))

	assert.Error(t, err)
}

func TestValidRef_AcceptsOnlyTheRefsRowsCarry(t *testing.T) {
	ref, err := newBlobs(t).Put([]byte("payload"))
	require.NoError(t, err)
	digest := strings.TrimPrefix(ref, ledgerstore.RefPrefix)

	assert.True(t, ledgerstore.ValidRef(ref), "a ref Put hands out")
	for name, bad := range map[string]string{
		"bare hex":     digest,
		"upper case":   ledgerstore.RefPrefix + strings.ToUpper(digest),
		"short":        ref[:len(ref)-1],
		"long":         ref + "0",
		"other prefix": "sha1:" + digest,
		"a path":       ledgerstore.RefPrefix + "../" + digest[3:],
		"empty":        "",
	} {
		assert.False(t, ledgerstore.ValidRef(bad), name)
	}
}
