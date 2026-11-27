package engine_test

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const enginePrefix = "github.com/char2cs/crowbar/api/internal/engine/"

func TestEngines_DoNotCrossImport(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	var checked int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		own := enginePrefix + e.Name()
		walkGoPackages(t, filepath.Join(root, e.Name()), func(dir string, pkg *build.Package) {
			checked++
			imports := append(append([]string{}, pkg.Imports...), pkg.TestImports...)
			for _, imp := range imports {
				if !strings.HasPrefix(imp, enginePrefix) {
					continue
				}
				if imp == own || strings.HasPrefix(imp, own+"/") {
					continue
				}
				rel, _ := filepath.Rel(root, dir)
				t.Errorf(
					"engine/%s imports %s — engines must not cross-import; promote the shared capability to core/",
					rel, imp,
				)
			}
		})
	}

	if checked == 0 {
		t.Fatal("the walk found no engine packages; this guard is not actually checking anything")
	}
}

func TestEngines_DoNotImportTheEventStoreAdapter(t *testing.T) {
	const forbidden = "github.com/char2cs/crowbar/api/internal/adapter/eventstore"

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	walkGoPackages(t, root, func(dir string, pkg *build.Package) {
		for _, imp := range pkg.Imports {
			if imp == forbidden || strings.HasPrefix(imp, forbidden+"/") {
				rel, _ := filepath.Rel(root, dir)
				t.Errorf("engine/%s imports %s — the engine declares the port, the app builds the instance",
					rel, imp)
			}
		}
	})
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
