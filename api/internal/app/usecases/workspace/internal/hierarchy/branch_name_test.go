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
