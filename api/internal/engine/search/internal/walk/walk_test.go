package walk_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/search/internal/walk"
)

// nopIgnorer accepts everything (never ignores).
type nopIgnorer struct{}

func (nopIgnorer) ShouldIgnore(_ string) bool { return false }

// listIgnorer ignores paths that are under a hard-coded suffix.
type listIgnorer struct {
	ignored []string
}

func (l *listIgnorer) ShouldIgnore(absPath string) bool {
	for _, s := range l.ignored {
		if absPath == s {
			return true
		}
	}
	return false
}

func buildTree(
	t *testing.T,
	root string,
	files []string,
) {
	t.Helper()
	for _, rel := range files {
		abs := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte("content"), 0o600))
	}
}

func collectAll(
	ctx context.Context,
	ch <-chan string,
) []string {
	var result []string
	for p := range ch {
		result = append(result, p)
	}
	return result
}

func TestWalk_ReturnsAllFiles(t *testing.T) {
	dir := t.TempDir()
	buildTree(t, dir, []string{"a.go", "b.go", "sub/c.go"})

	ch, err := walk.Walk(context.Background(), dir, nopIgnorer{}, nil, nil)
	require.NoError(t, err)
	got := collectAll(context.Background(), ch)
	sort.Strings(got)

	require.Len(t, got, 3)
}

func TestWalk_IgnorerExcludesPath(t *testing.T) {
	dir := t.TempDir()
	buildTree(t, dir, []string{"a.go", "vendor/lib.go"})

	ig := &listIgnorer{ignored: []string{filepath.Join(dir, "vendor", "lib.go")}}
	ch, err := walk.Walk(context.Background(), dir, ig, nil, nil)
	require.NoError(t, err)
	got := collectAll(context.Background(), ch)

	require.Len(t, got, 1)
	assert.Contains(t, got[0], "a.go")
}

func TestWalk_IncludeGlob(t *testing.T) {
	dir := t.TempDir()
	buildTree(t, dir, []string{"main.go", "main.ts", "lib.go"})

	ch, err := walk.Walk(context.Background(), dir, nopIgnorer{}, []string{"*.go"}, nil)
	require.NoError(t, err)
	got := collectAll(context.Background(), ch)
	sort.Strings(got)

	require.Len(t, got, 2)
	for _, p := range got {
		assert.Contains(t, p, ".go")
	}
}

func TestWalk_ExcludeGlob(t *testing.T) {
	dir := t.TempDir()
	buildTree(t, dir, []string{"main.go", "main_test.go", "lib.go"})

	ch, err := walk.Walk(context.Background(), dir, nopIgnorer{}, nil, []string{"*_test.go"})
	require.NoError(t, err)
	got := collectAll(context.Background(), ch)
	sort.Strings(got)

	require.Len(t, got, 2)
	for _, p := range got {
		assert.NotContains(t, p, "_test")
	}
}

func TestWalk_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	// Build enough files that cancellation can take effect.
	files := make([]string, 50)
	for i := range files {
		files[i] = filepath.Join("sub", "file"+string(rune('a'+i%26))+string(rune('0'+i/26))+".go")
	}
	buildTree(t, dir, files)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ch, err := walk.Walk(ctx, dir, nopIgnorer{}, nil, nil)
	require.NoError(t, err)
	// Just drain — we don't assert count, only that it doesn't hang.
	for range ch {
	}
}

func TestWalk_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ch, err := walk.Walk(context.Background(), dir, nopIgnorer{}, nil, nil)
	require.NoError(t, err)
	got := collectAll(context.Background(), ch)
	assert.Empty(t, got)
}

func TestWalk_InaccessibleRoot(t *testing.T) {
	_, err := walk.Walk(context.Background(), "/nonexistent/path/does/not/exist", nopIgnorer{}, nil, nil)
	require.Error(t, err)
}

func TestWalk_IncludeAndExclude(t *testing.T) {
	dir := t.TempDir()
	buildTree(t, dir, []string{"a.go", "b_test.go", "c.ts"})

	// Include *.go, exclude *_test.go.
	ch, err := walk.Walk(context.Background(), dir, nopIgnorer{}, []string{"*.go"}, []string{"*_test.go"})
	require.NoError(t, err)
	got := collectAll(context.Background(), ch)

	require.Len(t, got, 1)
	assert.Contains(t, got[0], "a.go")
}

func TestWalk_IgnoredDirectory_SkipsContents(t *testing.T) {
	dir := t.TempDir()
	buildTree(t, dir, []string{"a.go", "ignored_dir/b.go", "ignored_dir/c.go"})

	ignoredPath := filepath.Join(dir, "ignored_dir")
	ig := &listIgnorer{ignored: []string{ignoredPath}}

	ch, err := walk.Walk(context.Background(), dir, ig, nil, nil)
	require.NoError(t, err)
	got := collectAll(context.Background(), ch)

	require.Len(t, got, 1)
	assert.Contains(t, got[0], "a.go")
}

func TestWalk_PermissionDeniedSubdir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission denial has no effect")
	}
	dir := t.TempDir()
	buildTree(t, dir, []string{"a.go", "sub/b.go"})

	require.NoError(t, os.Chmod(filepath.Join(dir, "sub"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "sub"), 0o755) })

	ch, err := walk.Walk(context.Background(), dir, nopIgnorer{}, nil, nil)
	require.NoError(t, err)
	got := collectAll(context.Background(), ch)

	// At minimum, the top-level file is returned; sub is skipped.
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "a.go")
}
