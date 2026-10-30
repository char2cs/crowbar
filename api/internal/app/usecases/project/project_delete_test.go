package project_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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
	assert.ErrorIs(t, f.uc.Delete(context.Background(), "missing", domain.KeepWorkAtRisk), problem.ErrNotFound)
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

// A delete that stops is never silent (D5): the rows keep their durable intent,
// the error is the caller's, and boot finishes what PendingDeletes lists.
func TestPendingDeletes_ListsDeletesARestartInterrupted(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.cascade.err = errors.New("worktree wedged")
	ctx := context.Background()

	require.ErrorContains(t, f.uc.Delete(ctx, "p1", domain.KeepWorkAtRisk), "worktree wedged")
	assert.True(t, f.projects.projects["p1"].Deleting)
	assert.True(t, f.repos.row("r1").Deleting)

	pending, err := f.uc.PendingDeletes(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1, "the project's repo goes with its project")
	assert.Equal(t, domain.OpProjectDelete, pending[0].Kind)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityProject, ID: "p1"}, pending[0].Ref)
	assert.Equal(t, "Remove demo", pending[0].Label)

	f.cascade.err = nil
	require.NoError(t, pending[0].Run(ctx))
	assert.Equal(t, []string{"p1"}, f.projects.deleted)
	assert.Equal(t, []string{"r1"}, f.repos.deleted)
}

// A lone repo delete that stopped is listed on its own; live rows are not.
func TestPendingDeletes_ListsALoneRepoDelete(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()
	f.repos.repos[1].Deleting = true

	pending, err := f.uc.PendingDeletes(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityRepo, ID: "r-other"}, pending[0].Ref)
	assert.Equal(t, "p2", pending[0].ProjectID)
	require.NoError(t, pending[0].Run(context.Background()))
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
// its 202) is not saved again; a row not marked, as one a failure left behind
// before it was marked, records the intent first.
func TestDeleteRepo_AStoppedDeleteIsMarkedAgain(t *testing.T) {
	f := newDeleteFixture(t)
	require.NoError(t, f.uc.DeleteRepo(context.Background(),
		domain.Repository{ID: "r1", ProjectID: "p1", Deleting: true}, domain.KeepWorkAtRisk))
	assert.Zero(t, f.repos.saves, "a marked row is handed over as-is")

	g := newDeleteFixture(t)
	require.NoError(t, g.uc.DeleteRepo(context.Background(),
		domain.Repository{ID: "r1", ProjectID: "p1"}, domain.KeepWorkAtRisk))
	assert.Equal(t, 1, g.repos.saves, "an unmarked row is marked before it is torn down")
}

// A failed cascade keeps the row and returns why; nothing is written back to
// the row, since the caller's operation records the reason.
func TestDeleteRepo_AFailureIsReturnedNotSaved(t *testing.T) {
	f := newDeleteFixture(t)
	f.cascade.err = errors.New("boom")

	require.ErrorContains(t, f.uc.DeleteRepo(context.Background(), domain.Repository{ID: "r1", ProjectID: "p1"}, domain.KeepWorkAtRisk), "boom")
	assert.Equal(t, 1, f.repos.saves, "only the intent was saved")
	assert.True(t, f.repos.row("r1").Deleting)
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
	require.ErrorIs(t, err, domain.ErrWorkAtRisk)
	assert.Equal(t, map[string]any{"workAtRisk": []domain.WorkAtRisk{risk}}, problem.DataOf(err))

	_, err = f.uc.BeginDelete(context.Background(), "p1", domain.KeepWorkAtRisk)
	require.ErrorIs(t, err, domain.ErrWorkAtRisk)
	assert.Zero(t, f.repos.saves, "no repo intent recorded")
	assert.False(t, f.projects.projects["p1"].Deleting, "no project intent recorded")

	marked, err := f.uc.BeginRepoDelete(context.Background(), repo, domain.DiscardWorkAtRisk)
	require.NoError(t, err)
	assert.True(t, marked.Deleting)
}

// The consent a delete was given reaches the cascade; a resumed delete has none.
func TestDeleteRepo_HandsItsConsentToTheCascadeAndAResumedDeleteHasNone(t *testing.T) {
	f := newDeleteFixture(t)
	f.seedProject()

	require.NoError(t, f.uc.DeleteRepo(context.Background(), f.repos.repos[0], domain.DiscardWorkAtRisk))
	f.repos.repos = append(f.repos.repos, domain.Repository{ID: "r2", ProjectID: "p3", Deleting: true})
	pending, err := f.uc.PendingDeletes(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, pending[0].Run(context.Background()))

	assert.Equal(t, []domain.DeleteConsent{domain.DiscardWorkAtRisk, domain.KeepWorkAtRisk}, f.cascade.consents)
}
