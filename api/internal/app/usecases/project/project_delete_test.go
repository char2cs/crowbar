package project_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeDeleteProjects struct {
	projects map[string]domain.Project
	deleted  []string
	findErr  error
	delErr   error
}

func (f *fakeDeleteProjects) FindByKey(_ context.Context, id string) (*domain.Project, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	p, ok := f.projects[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (f *fakeDeleteProjects) FindAll(_ context.Context) ([]domain.Project, error) {
	out := make([]domain.Project, 0, len(f.projects))
	for _, p := range f.projects {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeDeleteProjects) Save(_ context.Context, p domain.Project) error {
	f.projects[p.ID] = p
	return nil
}

func (f *fakeDeleteProjects) Delete(_ context.Context, id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	delete(f.projects, id)
	return nil
}

type fakeDeleteRepos struct {
	repos   []domain.Repository
	saves   int
	deleted []string
	findErr error
	delErr  error
	log     *[]string
}

func (f *fakeDeleteRepos) FindAll(_ context.Context) ([]domain.Repository, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.repos, nil
}

func (f *fakeDeleteRepos) Save(_ context.Context, r domain.Repository) error {
	f.saves++
	for i := range f.repos {
		if f.repos[i].ID == r.ID {
			f.repos[i] = r
			return nil
		}
	}
	f.repos = append(f.repos, r)
	return nil
}

func (f *fakeDeleteRepos) row(id string) domain.Repository {
	for _, r := range f.repos {
		if r.ID == id {
			return r
		}
	}
	return domain.Repository{}
}

func (f *fakeDeleteRepos) Delete(_ context.Context, id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	for i := range f.repos {
		if f.repos[i].ID == id {
			f.repos = append(f.repos[:i], f.repos[i+1:]...)
			break
		}
	}
	if f.log != nil {
		*f.log = append(*f.log, "row:"+id)
	}
	return nil
}

type fakeDeleteWorkspaces struct {
	workspaces []domain.Workspace
	deleted    []string
	listErr    error
	delErr     error
}

func (f *fakeDeleteWorkspaces) List(_ context.Context) ([]domain.Workspace, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.workspaces, nil
}

func (f *fakeDeleteWorkspaces) Delete(_ context.Context, id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

// fakeRepoCascade stands in for hierarchy.DeleteRepoWorkspaces, recording the
// repos it was handed (and the order, against the record deletes).
type fakeRepoCascade struct {
	repos    []domain.Repository
	log      *[]string
	err      error
	risks    map[string][]domain.WorkAtRisk
	consents []domain.DeleteConsent
}

func (f *fakeRepoCascade) RepoWorkAtRisk(_ context.Context, repo domain.Repository) ([]domain.WorkAtRisk, error) {
	return f.risks[repo.ID], nil
}

func (f *fakeRepoCascade) DeleteRepoWorkspaces(
	_ context.Context,
	repo domain.Repository,
	consent domain.DeleteConsent,
) error {
	f.consents = append(f.consents, consent)
	if f.err != nil {
		return f.err
	}
	f.repos = append(f.repos, repo)
	*f.log = append(*f.log, "cascade:"+repo.ID)
	return nil
}

type fakeDeleteNodes struct{ forgot []string }

func (f *fakeDeleteNodes) Forget(_ context.Context, id string) error {
	f.forgot = append(f.forgot, id)
	return nil
}

type deleteFixture struct {
	uc         project.DeleteUsecase
	projects   *fakeDeleteProjects
	repos      *fakeDeleteRepos
	workspaces *fakeDeleteWorkspaces
	cascade    *fakeRepoCascade
	nodes      *fakeDeleteNodes
	home       string
	log        []string
}

func newDeleteFixture(t *testing.T) *deleteFixture {
	t.Helper()
	f := &deleteFixture{
		projects:   &fakeDeleteProjects{projects: map[string]domain.Project{}},
		repos:      &fakeDeleteRepos{},
		workspaces: &fakeDeleteWorkspaces{},
		nodes:      &fakeDeleteNodes{},
		home:       t.TempDir(),
	}
	f.cascade = &fakeRepoCascade{log: &f.log}
	f.repos.log = &f.log
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:       f.projects,
		Repos:          f.repos,
		Workspaces:     f.workspaces,
		RepoWorkspaces: f.cascade,
		Nodes:          f.nodes,
		CrowbarHome:    func() (string, error) { return f.home, nil },
	})
	return f
}

const deleteRepoPath = "/home/u/proj/repo"

func (f *deleteFixture) seedProject() {
	f.projects.projects["p1"] = domain.Project{ID: "p1", Name: "demo", Path: deleteRepoPath}
	f.repos.repos = append(f.repos.repos,
		domain.Repository{ID: "r1", ProjectID: "p1", Path: deleteRepoPath, DefaultBranch: "main"},
		domain.Repository{ID: "r-other", ProjectID: "p2", Path: "/elsewhere"},
	)
}

func TestProjectDelete_NotFound(t *testing.T) {
	f := newDeleteFixture(t)
	assert.ErrorIs(t, f.uc.Delete(context.Background(), "missing", domain.KeepWorkAtRisk), apperr.ErrNotFound)
}

// Every repo's workspaces go through the repo cascade — the one lifecycle path
// that never deletes a branch Crowbar did not create and never forces a locked
// worktree — handed the WHOLE repo, default branch included. The project's own
// home row, which no repo cascade takes, is tombstoned directly. Records go
// only once their workspaces are retired, and every Node row goes with them.
func TestProjectDelete_RetiresWorkspacesThroughTheRepoCascade(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-home", ProjectID: "p1", Kind: domain.WorkspaceKindHome, WorktreePath: deleteRepoPath, Provisioning: domain.WorkspaceShared},
		{ID: "w-r1", ProjectID: "p1", RepoID: "r1", Branch: "feature"},
		{ID: "w-other", ProjectID: "p2", RepoID: "r-other", Branch: "x"},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	require.Len(t, f.cascade.repos, 1)
	assert.Equal(t, "main", f.cascade.repos[0].DefaultBranch, "the cascade knows the branch it must keep")
	assert.Equal(t, []string{"w-home"}, f.workspaces.deleted,
		"only the repo-less home row is tombstoned here; repo rows were the cascade's")
	assert.Equal(t, []string{"r1"}, f.repos.deleted)
	assert.Equal(t, []string{"r1"}, f.nodes.forgot, "the repo's Node row goes with it (D5)")
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

// A failed cascade leaves every record in place: nothing is deleted before
// its workspaces are durably retired.
func TestProjectDelete_CascadeFailure_KeepsTheRecords(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.cascade.err = errors.New("boom")

	require.Error(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))
	assert.Empty(t, f.repos.deleted)
	assert.Empty(t, f.projects.deleted)
}

// A delete that stops is never silent (D5): the rows keep their durable intent
// and say why, and Resume — run at boot — finishes them.
func TestProjectDelete_AStoppedDeleteIsRecordedAndResumed(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.cascade.err = errors.New("worktree wedged")
	ctx := context.Background()

	require.Error(t, f.uc.Delete(ctx, "p1", domain.KeepWorkAtRisk))
	p := f.projects.projects["p1"]
	assert.True(t, p.Deleting)
	assert.Contains(t, p.LastError, "worktree wedged")
	r := f.repos.row("r1")
	assert.True(t, r.Deleting)
	assert.Contains(t, r.LastError, "worktree wedged")

	f.cascade.err = nil
	require.NoError(t, f.uc.Resume(ctx))
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
	assert.Equal(t, []string{"r1"}, f.repos.deleted)
	assert.NotContains(t, f.projects.projects, "p1")
}

// A lone repo delete that stopped is resumed on its own; live rows are not
// touched.
func TestProjectDelete_ResumeFinishesALoneRepoDelete(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.repos.repos[1].Deleting = true
	f.repos.repos[1].LastError = "earlier failure"

	require.NoError(t, f.uc.Resume(context.Background()))
	assert.Equal(t, []string{"r-other"}, f.repos.deleted)
	assert.Empty(t, f.projects.deleted)
}

func TestProjectDelete_ListWorkspacesError_Aborts(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.listErr = errors.New("boom")
	require.Error(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))
	assert.Empty(t, f.projects.deleted)
}

func TestProjectDelete_RepoRecordDeleteError_AbortsBeforeProjectRow(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.repos.delErr = errors.New("boom")
	require.Error(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))
	assert.Empty(t, f.projects.deleted)
}

func TestProjectDelete_ProjectRecordDeleteError_Surfaces(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.projects.delErr = errors.New("boom")
	require.Error(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))
}

func TestProjectDelete_FindProjectError_Aborts(t *testing.T) {
	f := newDeleteFixture(t)
	f.projects.findErr = errors.New("boom")
	require.Error(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))
}

func TestProjectDelete_ListReposError_Aborts(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.repos.findErr = errors.New("boom")
	require.Error(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))
	assert.Empty(t, f.projects.deleted)
}

// The project's whole tree goes: icon, repo entity dirs, worktree roots.
func TestProjectDelete_RemovesTheProjectTree(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	projectDir := filepath.Join(f.home, "projects", "p1")
	root := filepath.Join(projectDir, "github.com", "acme", "repo", "feature")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktree"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "chats", "c1"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "r1", "storages"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "icon"), []byte("png"), 0o644))

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	assert.NoDirExists(t, projectDir, "projects/p1 must be gone entirely")
}

// A repo moved to project p2 keeps its worktrees where they were created, under
// projects/p1. Deleting p1 used to rm -rf them — p2's worktrees and chats — with
// it (spec §3 P0-3, invariant D6). They, and only they, survive; so does a
// row whose own ProjectID still says p1 but whose repo is p2's.
func TestRegression_ProjectDelete_NeverRemovesAnotherProjectsWorktree(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	projectDir := filepath.Join(f.home, "projects", "p1")
	moved := filepath.Join(projectDir, "github.com", "acme", "other", "b", "worktree")
	stale := filepath.Join(projectDir, "github.com", "acme", "other", "c", "worktree")
	mine := filepath.Join(projectDir, "github.com", "acme", "repo", "feature", "worktree")
	for _, dir := range []string{moved, stale, mine} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "work.txt"), []byte("w"), 0o644))
	}
	movedChats := filepath.Join(filepath.Dir(moved), "chats", "c1")
	require.NoError(t, os.MkdirAll(movedChats, 0o755))
	// A pre-leaf row of the moved repo: its checkout and the shared chats tree.
	preLeaf := filepath.Join(projectDir, "github.com", "acme", "other", "d")
	preLeafChats := filepath.Join(projectDir, "github.com", "acme", "other", "chats", "c2")
	require.NoError(t, os.MkdirAll(preLeaf, 0o755))
	require.NoError(t, os.MkdirAll(preLeafChats, 0o755))
	f.workspaces.workspaces = []domain.Workspace{
		{ID: "w-preleaf", ProjectID: "p2", RepoID: "r-other", WorktreePath: preLeaf, Provisioning: domain.WorkspaceProvisioned},
		{ID: "w-moved", ProjectID: "p2", RepoID: "r-other", WorktreePath: moved, Provisioning: domain.WorkspaceProvisioned},
		{ID: "w-stale", ProjectID: "p1", RepoID: "r-other", WorktreePath: stale, Provisioning: domain.WorkspaceProvisioned},
		{ID: "w-mine", ProjectID: "p1", RepoID: "r1", WorktreePath: mine, Provisioning: domain.WorkspaceProvisioned},
	}

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	assert.FileExists(t, filepath.Join(moved, "work.txt"), "another project's worktree survives")
	assert.DirExists(t, movedChats, "with the chats beside it")
	assert.DirExists(t, preLeaf, "a pre-leaf checkout of another project survives")
	assert.DirExists(t, preLeafChats, "with the chats tree it resolves")
	assert.FileExists(t, filepath.Join(stale, "work.txt"), "a row of another project's repo survives")
	assert.NoDirExists(t, mine, "the project's own worktree goes")
	assert.NotContains(t, f.workspaces.deleted, "w-stale", "and its row is not the project's to tombstone")
}

// A worktree git refused to remove — a protected one with uncommitted work —
// is still a registered checkout. It is not the project delete's to rm -rf:
// that would destroy the work and strand the registration in the user's repo.
func TestRegression_ProjectDelete_KeepsACheckoutGitStillRegisters(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	worktree := filepath.Join(f.home, "projects", "p1", "github.com", "acme", "repo", "main", "worktree")
	require.NoError(t, os.MkdirAll(worktree, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "unsaved.txt"), []byte("w"), 0o644))

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	assert.FileExists(t, filepath.Join(worktree, "unsaved.txt"))
}

// The user's real repository lives outside the crowbar home and is never
// touched, whatever the rows say.
func TestProjectDelete_NeverTouchesTheRealRepoPath(t *testing.T) {
	f := newDeleteFixture(t)
	real := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(real, "README.md"), []byte("hi"), 0o644))
	f.projects.projects["p1"] = domain.Project{ID: "p1", Path: real}
	f.repos.repos = []domain.Repository{{ID: "r1", ProjectID: "p1", Path: real}}
	f.workspaces.workspaces = []domain.Workspace{{ID: "w-home", ProjectID: "p1", WorktreePath: real, Provisioning: domain.WorkspaceProvisioned}}

	require.NoError(t, f.uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))

	assert.FileExists(t, filepath.Join(real, "README.md"))
}

func TestProjectDelete_NoCrowbarHome_SkipsDiskTeardown(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	uc := project.NewDelete(project.DeleteDeps{
		Projects: f.projects, Repos: f.repos, Workspaces: f.workspaces,
		RepoWorkspaces: f.cascade, Nodes: f.nodes,
		CrowbarHome: func() (string, error) { return "", errors.New("no home") },
	})
	require.NoError(t, uc.Delete(context.Background(), "p1", domain.KeepWorkAtRisk))
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
}

// A repo delete retires its workspaces BEFORE its row goes: deleting the row
// first let a crash strand workspaces whose repo no longer resolves, and ran
// their teardown without the default branch (spec §3 P0-1). Its Node row and
// its entity directory go with it.
func TestDeleteRepo_RetiresWorkspacesBeforeTheRow(t *testing.T) {
	f := newDeleteFixture(t)
	repoDir := filepath.Join(f.home, "projects", "p1", "r1")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "storages"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "icon"), []byte("png"), 0o644))

	require.NoError(t, f.uc.DeleteRepo(context.Background(), domain.Repository{ID: "r1", ProjectID: "p1"}, domain.KeepWorkAtRisk))

	assert.Equal(t, []string{"cascade:r1", "row:r1"}, f.log)
	assert.Equal(t, []string{"r1"}, f.nodes.forgot)
	assert.NoDirExists(t, repoDir)
}

// A row the caller already marked (the HTTP handler records the intent before
// its 202) is not saved again; an unmarked row, or one carrying a previous
// attempt's error, records the intent first.
func TestDeleteRepo_RecordsTheIntentOnlyWhenNotYetRecorded(t *testing.T) {
	f := newDeleteFixture(t)
	require.NoError(t, f.uc.DeleteRepo(context.Background(),
		domain.Repository{ID: "r1", ProjectID: "p1", Deleting: true}, domain.KeepWorkAtRisk))
	assert.Zero(t, f.repos.saves, "a marked row is handed over as-is")

	g := newDeleteFixture(t)
	require.NoError(t, g.uc.DeleteRepo(context.Background(),
		domain.Repository{ID: "r1", ProjectID: "p1", Deleting: true, LastError: "earlier"}, domain.KeepWorkAtRisk))
	assert.Equal(t, 1, g.repos.saves, "a retry clears the previous error before tearing down")
}

// A cascade that cannot list the repo's workspaces keeps the row: the caller
// reports the repo as still present rather than half-deleted.
func TestDeleteRepo_CascadeFailure_KeepsTheRow(t *testing.T) {
	f := newDeleteFixture(t)
	f.cascade.err = errors.New("boom")

	require.Error(t, f.uc.DeleteRepo(context.Background(), domain.Repository{ID: "r1", ProjectID: "p1"}, domain.KeepWorkAtRisk))
	assert.Empty(t, f.repos.deleted)
	assert.Empty(t, f.nodes.forgot)
}

// Without consent, work at risk refuses a repo or project delete before its
// intent is recorded — nothing is marked, so boot has nothing to resume.
func TestBeginDelete_WithoutConsentRefusesOverWorkAtRiskBeforeRecordingIntent(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	risk := domain.WorkAtRisk{WorkspaceID: "w1", Branch: "feature/x", UnmergedCommits: 1}
	f.cascade.risks = map[string][]domain.WorkAtRisk{"r1": {risk}}
	repo := f.repos.repos[0]

	_, err := f.uc.BeginRepoDelete(context.Background(), repo, domain.KeepWorkAtRisk)
	var refused *domain.WorkAtRiskError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, []domain.WorkAtRisk{risk}, refused.Workspaces)

	_, err = f.uc.BeginDelete(context.Background(), "p1", domain.KeepWorkAtRisk)
	require.ErrorIs(t, err, domain.ErrWorkAtRisk)
	assert.Zero(t, f.repos.saves, "no repo intent recorded")
	assert.False(t, f.projects.projects["p1"].Deleting, "no project intent recorded")

	marked, err := f.uc.BeginRepoDelete(context.Background(), repo, domain.DiscardWorkAtRisk)
	require.NoError(t, err)
	assert.True(t, marked.Deleting)
}

// The consent a delete was given reaches the cascade; a resumed delete has none.
func TestDeleteRepo_HandsItsConsentToTheCascadeAndResumeHasNone(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()

	require.NoError(t, f.uc.DeleteRepo(context.Background(), f.repos.repos[0], domain.DiscardWorkAtRisk))
	f.repos.repos = append(f.repos.repos, domain.Repository{ID: "r2", ProjectID: "p1", Deleting: true})
	require.NoError(t, f.uc.Resume(context.Background()))

	assert.Equal(t, []domain.DeleteConsent{domain.DiscardWorkAtRisk, domain.KeepWorkAtRisk}, f.cascade.consents)
}
