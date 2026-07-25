package worktree

import "testing"

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestChainFor_BuildsAncestorsFirstStoppingAtTerminals(t *testing.T) {
	base := map[string]string{"feat/9324": "feat/base", "feat/base": "dev"}

	// Nothing imported yet: the full chain is created, dev (default) is the terminal.
	got := chainFor("feat/9324", "dev", base, map[string]string{})
	want := []string{"feat/base", "feat/9324"}
	if !slicesEqual(got, want) {
		t.Fatalf("chain = %v, want %v", got, want)
	}

	// An existing ancestor is a terminal: feat/base already a workspace.
	got2 := chainFor("feat/9324", "dev", base, map[string]string{"feat/base": "ws-base"})
	if !slicesEqual(got2, []string{"feat/9324"}) {
		t.Fatalf("chain2 = %v, want [feat/9324]", got2)
	}

	// A branch with no PR base terminates immediately at the default branch.
	got3 := chainFor("feat/orphan", "dev", base, map[string]string{})
	if !slicesEqual(got3, []string{"feat/orphan"}) {
		t.Fatalf("chain3 = %v, want [feat/orphan]", got3)
	}

	// Cycle guard: base points back up; the leaf must still be included.
	baseCyc := map[string]string{"a": "b", "b": "a"}
	got4 := chainFor("a", "main", baseCyc, map[string]string{})
	if len(got4) == 0 || got4[len(got4)-1] != "a" {
		t.Fatalf("cycle chain must still include the leaf: %v", got4)
	}

	// The default branch itself is never a node to create.
	got5 := chainFor("dev", "dev", base, map[string]string{})
	if len(got5) != 0 {
		t.Fatalf("default branch must yield an empty chain: %v", got5)
	}
}
