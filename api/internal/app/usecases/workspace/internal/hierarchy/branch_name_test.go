package hierarchy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// TestCreateChild_BlankBranch_GeneratesAndChecksAgainstRealRefs proves the
// model spec §4.1 create-time generator: a caller (Promote) that leaves
// Branch blank gets a server-minted name, and CreateChild actually consulted
// the repo's real refs to pick it rather than trusting a bare random draw.
func TestCreateChild_BlankBranch_GeneratesAndChecksAgainstRealRefs(t *testing.T) {
	g := &fakeGit{addStartSha: "sha", branches: []gitdomain.Branch{{Name: "main"}}}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, created.Branch, "a blank Branch must be filled in, not left empty")
	assert.Contains(t, g.ops(), "Branches", "the generator must consult the repo's real refs")
}

// TestCreateChild_BlankBranch_RetriesOnARealCollision proves the generator
// does not just draw a random name and hope: a candidate that collides with a
// real ref is discarded and another is drawn.
func TestCreateChild_BlankBranch_RetriesOnARealCollision(t *testing.T) {
	g := &fakeGit{addStartSha: "sha", branches: []gitdomain.Branch{{Name: "chat-taken"}}}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	calls := 0
	names := []string{"chat-taken", "chat-taken", "chat-free"}
	restore := hierarchy.SetBranchNameCandidateForTest(func() string {
		name := names[calls]
		calls++
		return name
	})
	defer restore()
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})

	require.NoError(t, err)
	assert.Equal(t, "chat-free", created.Branch, "a colliding candidate must be discarded, not used")
	assert.Equal(t, 3, calls, "must retry past both collisions before landing on the free name")
}

// TestCreateChild_BlankBranch_DetectsARemotePrefixedCollision proves the
// collision check matches a remote ref ("origin/chat-taken") against its bare
// local name, since `git branch -a` lists remotes with that prefix.
func TestCreateChild_BlankBranch_DetectsARemotePrefixedCollision(t *testing.T) {
	g := &fakeGit{addStartSha: "sha", branches: []gitdomain.Branch{{Name: "origin/chat-taken", IsRemote: true}}}
	var created workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	calls := 0
	names := []string{"chat-taken", "chat-free"}
	restore := hierarchy.SetBranchNameCandidateForTest(func() string {
		name := names[calls]
		calls++
		return name
	})
	defer restore()
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})

	require.NoError(t, err)
	assert.Equal(t, "chat-free", created.Branch)
}

// TestCreateChild_BlankBranch_GivesUpAfterTooManyCollisions proves the
// generator fails loudly rather than looping forever or silently minting a
// colliding branch.
func TestCreateChild_BlankBranch_GivesUpAfterTooManyCollisions(t *testing.T) {
	g := &fakeGit{addStartSha: "sha", branches: []gitdomain.Branch{{Name: "chat-taken"}}}
	ws := &fakeWorkspace{}
	restore := hierarchy.SetBranchNameCandidateForTest(func() string { return "chat-taken" })
	defer restore()
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})

	require.Error(t, err)
}

// TestCreateChild_ExplicitBranch_NeverConsultsTheGenerator proves every
// existing explicit-create caller is untouched: supplying Branch skips the
// generator (and its Branches lookup) entirely.
func TestCreateChild_ExplicitBranch_NeverConsultsTheGenerator(t *testing.T) {
	g := &fakeGit{addStartSha: "sha"}
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), hierarchy.CreateChildInput{
		RepoID:       "r1",
		ProjectID:    "p1",
		RepoPath:     "/repo",
		RemoteURL:    "https://github.com/test/repo.git",
		Branch:       "feature/x",
		ParentID:     "w-parent",
		ParentBranch: "develop",
	})

	require.NoError(t, err)
	assert.NotContains(t, g.ops(), "Branches")
}
