package repositories_test

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The agent repositories ANNOUNCE events. They must never reach the frontend: deciding
// what a client is told is the usecase's job, and lives in usecases/agent's fanout
// (spec 2026-08-23 §1.5). This guard is the reason that stays true — without it the
// next person adds a broadcast back into a projection and nothing complains.
//
// Scope is deliberately repositories/chat (which now contains activity too). The
// runner aggregate moved into engine/agents/runner and is covered by the engine's own
// guards; the workspace aggregate still broadcasts from this layer
// (container.go:177, :504), and widening this test would fail on day one for a reason
// that has nothing to do with chats.
func TestChatRepository_DoesNotReachTheFrontend(t *testing.T) {
	forbidden := []string{
		"github.com/char2cs/crowbar/api/internal/app/hub",
		"github.com/char2cs/crowbar/api/internal/app/realtime",
		"github.com/char2cs/crowbar/api/internal/api",
	}

	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}

	for _, agg := range []string{"chat"} {
		aggRoot := filepath.Join(root, agg)
		if _, err := os.Stat(aggRoot); err != nil {
			continue // folded into another package by a later stage
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
