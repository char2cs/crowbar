package avatar

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaletteSizeMatchesConst(t *testing.T) {
	assert.Len(t, Palette(), paletteSize)
}

func TestLabel_FirstAlnumUppercased(t *testing.T) {
	assert.Equal(t, "C", Label("crowbar"))
	assert.Equal(t, "9", Label("9front"))
	assert.Equal(t, "A", Label("  api"))
	assert.Equal(t, "?", Label(""))
	assert.Equal(t, "?", Label("---"))
}

func TestColor_StableForSameName(t *testing.T) {
	a := Color("crowbar")
	b := Color("crowbar")
	assert.Equal(t, a, b)
	assert.Contains(t, Palette(), a)
}

func TestColor_DistributesAcrossPalette(t *testing.T) {
	seen := map[string]bool{}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		seen[Color(n)] = true
	}
	assert.GreaterOrEqual(t, len(seen), 2)
}

func TestScanRepoIcon_FindsFaviconSVG(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644))
	got := ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(dir, "favicon.svg"), got)
}

func TestScanRepoIcon_PriorityOrder(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.ico"), []byte("ico"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644))
	got := ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(dir, "favicon.svg"), got) // svg wins over ico
}

func TestScanRepoIcon_PublicSubdir(t *testing.T) {
	dir := t.TempDir()
	pub := filepath.Join(dir, "public")
	require.NoError(t, os.Mkdir(pub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pub, "logo.png"), []byte("png"), 0o644))
	got := ScanRepoIcon(dir)
	assert.Equal(t, filepath.Join(pub, "logo.png"), got)
}

func TestScanRepoIcon_NoMatch_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got := ScanRepoIcon(dir)
	assert.Empty(t, got)
}
