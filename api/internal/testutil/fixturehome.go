package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// FixtureHome creates an empty Crowbar home under the test's temp dir and
// returns it, so a test never resolves to the real home.
func FixtureHome(tb testing.TB) home.Home {
	tb.Helper()
	root := filepath.Join(tb.TempDir(), "home")
	if err := os.MkdirAll(root, 0o700); err != nil {
		tb.Fatalf("fixture home: create %s: %v", root, err)
	}
	h, err := home.At(root)
	if err != nil {
		tb.Fatalf("fixture home: %v", err)
	}
	return h
}

// WriteFile writes content at rel under the home, creating parent
// directories with mode 0o700 and the file with mode 0o600.
func WriteFile(
	tb testing.TB,
	h home.Home,
	rel string,
	content string,
) {
	tb.Helper()
	path := filepath.Join(h.Root(), rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		tb.Fatalf("write %s: create parents: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		tb.Fatalf("write %s: %v", rel, err)
	}
}

// TreeSnapshot lists every file under the home as "relpath size", sorted, so
// a test can assert that a code path left the tree untouched.
func TreeSnapshot(
	tb testing.TB,
	h home.Home,
) []string {
	tb.Helper()
	snapshot := []string{}
	err := filepath.WalkDir(h.Root(), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(h.Root(), path)
		if err != nil {
			return err
		}
		snapshot = append(snapshot, filepath.ToSlash(rel)+" "+strconv.FormatInt(info.Size(), 10))
		return nil
	})
	if err != nil {
		tb.Fatalf("snapshot %s: %v", h.Root(), err)
	}
	sort.Strings(snapshot)
	return snapshot
}
