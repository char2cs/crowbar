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

// TestResolveImportParent covers the decision the import dialog's whole value
// rests on: an imported branch hangs off the workspace for the branch its PR
// targets, and off the default branch's workspace when there is no PR.
//
// The regression it pins: `base == defaultBranch` used to short-circuit to an
// empty ParentID — the repo home — BEFORE the existing-workspace lookup ran. A
// PR into develop/main, which is nearly every PR, therefore landed its branch at
// the REPO ROOT as a sibling of the branch it is based on. The lookup would have
// succeeded: repo import gives every protected branch, the default included, its
// own locked managed workspace, and existingBranchWorkspaces excludes only the
// repo home. Only a PR into a non-default protected branch parented correctly.
func TestResolveImportParent(t *testing.T) {
	const dev = "dev"
	existing := map[string]string{
		dev:            "ws-dev",     // the default branch's LOCKED managed workspace
		"release/1.x":  "ws-release", // another protected branch, also locked
		"feat/adopted": "ws-adopted", // an ordinary branch imported earlier
	}

	cases := []struct {
		name       string
		branch     string
		base       map[string]string
		existing   map[string]string
		created    map[string]string
		wantID     string
		wantBranch string
	}{{
		name:       "PR into the default branch nests under its locked workspace",
		branch:     "feat/x",
		base:       map[string]string{"feat/x": dev},
		existing:   existing,
		wantID:     "ws-dev",
		wantBranch: dev,
	}, {
		name:       "no PR at all also nests under the default branch's workspace",
		branch:     "feat/x",
		base:       map[string]string{},
		existing:   existing,
		wantID:     "ws-dev",
		wantBranch: dev,
	}, {
		name:       "PR into a non-default locked branch nests under THAT branch",
		branch:     "feat/x",
		base:       map[string]string{"feat/x": "release/1.x"},
		existing:   existing,
		wantID:     "ws-release",
		wantBranch: "release/1.x",
	}, {
		name:       "PR into a branch created earlier in the same batch",
		branch:     "feat/leaf",
		base:       map[string]string{"feat/leaf": "feat/mid"},
		existing:   existing,
		created:    map[string]string{"feat/mid": "ws-mid"},
		wantID:     "ws-mid",
		wantBranch: "feat/mid",
	}, {
		name:       "PR into a base nobody has imported falls back to the default workspace",
		branch:     "feat/x",
		base:       map[string]string{"feat/x": "feat/never-imported"},
		existing:   existing,
		wantID:     "ws-dev",
		wantBranch: dev,
	}, {
		// The repo home is the ONLY remaining home for a branch, and only when the
		// default branch has no workspace of its own (a provider failure at repo
		// import left it unprovisioned).
		name:       "no default-branch workspace is the one case that parents at the repo root",
		branch:     "feat/x",
		base:       map[string]string{"feat/x": dev},
		existing:   map[string]string{},
		wantID:     "",
		wantBranch: dev,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created := tc.created
			if created == nil {
				created = map[string]string{}
			}
			gotID, gotBranch := resolveImportParent(tc.branch, dev, tc.base, tc.existing, created)
			if gotID != tc.wantID {
				t.Errorf("parentID = %q, want %q", gotID, tc.wantID)
			}
			if gotBranch != tc.wantBranch {
				t.Errorf("parentBranch = %q, want %q", gotBranch, tc.wantBranch)
			}
		})
	}
}
