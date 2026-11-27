package repositories_test

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatRepository_DoesNotReachTheFrontend(t *testing.T) {
	forbidden := []string{
		"github.com/char2cs/crowbar/api/internal/app/stream",
		"github.com/char2cs/crowbar/api/internal/api",
	}

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	for _, agg := range []string{"chat"} {
		aggRoot := filepath.Join(root, agg)
		if _, err := os.Stat(aggRoot); err != nil {
			continue
		}
		walkGoPackages(t, aggRoot, func(dir string, pkg *build.Package) {
			imports := append(append([]string{}, pkg.Imports...), pkg.TestImports...)
			for _, imp := range imports {
				for _, bad := range forbidden {
					if imp != bad && !strings.HasPrefix(imp, bad+"/") {
						continue
					}
					rel, _ := filepath.Rel(root, dir)
					t.Errorf(
						"repositories/%s imports %s — the chat repository must announce, not broadcast",
						rel, imp,
					)
				}
			}
		})
	}
}

func walkGoPackages(t *testing.T, root string, fn func(string, *build.Package)) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		pkg, perr := build.ImportDir(path, 0)
		if perr != nil {
			return nil // no buildable Go files in this directory
		}
		fn(path, pkg)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
