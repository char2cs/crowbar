//go:build integration

package tests

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
)

// importedRepo bundles the ids a project import yields: the project, its
// discovered repository, and the workspace adopted from the repo's main
// worktree (the on-disk repo path).
//
// chatID is the chat that OWNS workspaceID — the id every chat-scoped route
// addresses that worktree through, now that the whole `/workspaces/:wsId/...`
// group is gone. It is bundled here rather than resolved per call site because
// the import mints it: every worktree is born under a chat, so the id is
// already on the chat list these fixtures read.
type importedRepo struct {
	projectID   string
	repoID      string
	workspaceID string
	chatID      string
	repoPath    string
}

// chatRow is one row of GET .../repos/:repoId/chats — the ONE read that answers
// everything the deleted workspace list used to, since a worktree-owning chat
// carries its git state inline (spec §5).
type chatRow struct {
	ID          string        `json:"id"`
	WorkspaceID string        `json:"workspaceId"`
	Type        string        `json:"type"`
	Worktree    *worktreeWire `json:"worktree"`
}

// worktreeWire is dto.ChatWorktreeDTO: the git half a worktree-owning chat
// carries, projected from the very same WorkspaceDTO the deleted workspace
// stream used to send.
type worktreeWire struct {
	Branch          string `json:"branch"`
	Status          string `json:"status"`
	Working         bool   `json:"working"`
	LastError       string `json:"lastError"`
	IsDefault       bool   `json:"isDefault"`
	Added           int    `json:"added"`
	Deleted         int    `json:"deleted"`
	MergeStrategy   string `json:"mergeStrategy"`
	CanMergeLocally bool   `json:"canMergeLocally"`
	MergeConflicts  bool   `json:"mergeConflicts"`
	ParentBranch    string `json:"parentBranch"`
	PRUrl           string `json:"prUrl"`
	PRTitle         string `json:"prTitle"`
	PRTargetBranch  string `json:"prTargetBranch"`
	LocalPath       string `json:"localPath"`
	HeldByPath      string `json:"heldByPath"`
	ForkPointSha    string `json:"forkPointSha"`
	ParentID        string `json:"parentId"`
	OwningChatID    string `json:"owningChatId"`
}

// listChats reads the repo's chat rows over the real REST surface.
func listChats(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
) []chatRow {
	t.Helper()
	var chats []chatRow
	h.get("/v0/projects/"+projectID+"/repos/"+repoID+"/chats", &chats)
	return chats
}

// importProject creates a real git repo and brings it into Crowbar via the
// two-step public flow: POST /v0/projects creates the project (+ its home
// workspace) and POST /v0/projects/:p/repos adds the repo (ImportRepo).
//
// Under the protected-branch provisioning model the home is NO LONGER
// force-detached: left on "main" it would stay the home AND surface "main" as a
// worktree-less placeholder, yielding two branch=="main" rows. To hand these
// tests a real LOCKED MANAGED "main" worktree unambiguously, the fixture detaches
// the repo's HEAD BEFORE import. The home is then adopted detached (branch==""),
// "main" is free, and ImportRepo provisions it as its own managed locked worktree
// under the crowbar home. The returned workspaceID is that managed locked "main"
// worktree (branch=="main", status "locked", real on-disk worktree at
// <home>/projects/<P>/<R>/workspaces/<W>/worktree); write-path tests use
// importWritableWorkspace to get an unlocked child forked off it. Tests that need
// the home to STAY on its protected branch (and thus a placeholder) use
// importProjectHomeHoldsDefault instead. All ids are learned from the
// hierarchical WS streams (dial-before-POST).
func importProject(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t)
	// Detach the home off "main" so the protected default branch is free and
	// provisions as its own managed locked worktree — the workspace these tests
	// operate on. (importProjectHomeHoldsDefault skips this to keep the home on
	// "main" and exercise the placeholder path instead.)
	runGit(t, repoPath, "checkout", "--detach")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	// The locked managed worktree for the protected default branch ("main") is
	// persisted during the background ImportRepo job. It is READ BACK rather than
	// awaited on a stream, and that is forced: a chat frame is only reachable
	// through a repo-scoped mount, and the repo whose id that mount needs is
	// created BY this same job — so by the moment there is a repoId to dial, the
	// adoption it would have carried has already happened. Quiesce drains the
	// projections and the chat list is then read; both are real signals, neither
	// is a poll or a sleep.
	//
	// With the home detached, "main" is unambiguously that single managed
	// worktree (the home carries branch==""), and it has a real on-disk path —
	// which is what tells it apart from the held PLACEHOLDER row the other
	// fixture below is about.
	h.Quiesce()
	chats := listChats(t, h, projectID, repoID)
	var wsID, chatID string
	for _, c := range chats {
		if c.Worktree == nil || c.Worktree.Branch != "main" || c.Worktree.HeldByPath != "" {
			continue
		}
		wsID, chatID = c.WorkspaceID, c.ID
		break
	}
	require.NotEmpty(t, wsID, "import must provision the main managed worktree")
	require.NotEmpty(t, chatID, "the main managed worktree must be born under a chat")

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: wsID,
		chatID:      chatID,
		repoPath:    repoPath,
	}
}

// importProjectHomeHoldsDefault imports a repo whose home STAYS on its protected
// default branch ("main"): unlike importProject it does NOT detach the home, so
// the home is adopted as the single isDefault workspace on "main" AND that same
// branch is surfaced as a locked, worktree-less PLACEHOLDER held by the repo
// folder (spec §3.4). It waits until BOTH the home and the placeholder have
// materialised so the placeholder-asserting tests never race the async import.
// The returned workspaceID is the home (isDefault) row; these tests read the
// repo's workspace list directly rather than operating on a managed worktree.
func importProjectHomeHoldsDefault(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t) // home stays on "main" (no detach)

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	// Read back rather than awaited on a stream, for the reason importProject
	// gives: the repo this fixture would have to dial is created by the very job
	// whose output it wants. Quiesce is the barrier.
	h.Quiesce()
	var homeID, homeChatID string
	sawPlaceholder := false
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree == nil {
			continue
		}
		if c.Worktree.IsDefault {
			homeID, homeChatID = c.WorkspaceID, c.ID
		}
		if c.Worktree.Branch == "main" && c.Worktree.HeldByPath != "" {
			sawPlaceholder = true
		}
	}
	require.NotEmpty(t, homeID, "import must adopt the home as the isDefault workspace")
	require.True(t, sawPlaceholder, "the held default branch must surface as a placeholder row")

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: homeID,
		chatID:      homeChatID,
		repoPath:    repoPath,
	}
}

// createProjectAndRepo runs the two-step public import flow (POST /v0/projects
// then POST .../repos) for an on-disk repo at repoPath and returns the project
// and repo ids learned from the hierarchical WS streams (dial-before-POST).
func createProjectAndRepo(
	t *testing.T,
	h *harness,
	repoPath string,
) (projectID, repoID string) {
	t.Helper()
	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "demo", "path": repoPath}, http.StatusAccepted)
	_ = resp.Body.Close()

	project := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == repoPath
	})
	projectID, _ = project["id"].(string)
	require.NotEmpty(t, projectID, "create must broadcast a ProjectDTO with an id")

	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	addResp := h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": "demo", "path": repoPath}, http.StatusAccepted)
	_ = addResp.Body.Close()
	repo := readUntil(t, reposWS, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ = repo["id"].(string)
	require.NotEmpty(t, repoID, "add-repo must broadcast a RepoDTO with an id")
	return projectID, repoID
}

// importWritableWorkspace imports a project and then creates a child workspace
// on a fresh, non-protected branch. The adopted main worktree is locked because
// "main" is a default protected branch (04 §5, 05 §3/§4), so every file/git
// write against it now rejects with 409; write flows must target an unlocked
// workspace. The returned id points at that unlocked child; projectID/repoID
// carry over from the import. Workspace creation is async (202 + WS): the id is
// learned from the WorkspaceDTO{status:"new"} on the repo-scoped stream.
func importWritableWorkspace(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	imported := importProject(t, h)
	imported.workspaceID, imported.chatID = createWorktree(t, h, imported, "feature/write", imported.workspaceID)
	return imported
}

// createWorktree cuts a worktree on a CALLER-NAMED branch under parentID and
// returns its workspace id together with the chat that owns it.
//
// It goes through the USECASE, not HTTP, because there is no longer an HTTP way
// to ask: spec §8 step 6 deleted the whole `workspaces` endpoint group, and the
// chat-scoped surface that replaced it only forks with an auto-derived branch
// name or imports a branch that already exists. Naming the branch is a FIXTURE
// need, not a product one — a test that asserts on "feature/write" has to be
// able to say "feature/write" — so it is met at the layer that still offers it
// rather than by adding a product capability nothing ships.
//
// CreateChild is the same call the live import path makes
// (usecases.worktreeChildCreator.CreateImportedWorkspace), with the same
// arguments, so a fixture worktree is born exactly as a real one is — owning
// chat included. Only the caller differs.
func createWorktree(
	t *testing.T,
	h *harness,
	imported importedRepo,
	branch string,
	parentID string,
) (wsID string, chatID string) {
	t.Helper()
	ctx := context.Background()

	repo, err := h.app.GORM.Repositories.FindByKey(ctx, imported.repoID)
	require.NoError(t, err, "createWorktree: read repo")
	require.NotNil(t, repo, "createWorktree: repo must exist")

	// ParentBranch is the START POINT the worktree is cut from, and it must be
	// supplied rather than left to resolveInherited: that defaulting only runs for
	// a caller naming NEITHER RepoID nor RepoPath, and this one names both (as the
	// live import path does). Left blank, `git worktree add` rev-parses "" and dies.
	parentBranch := repo.DefaultBranch
	if parentID != "" {
		parent, perr := h.app.Repositories.Workspace.Get(ctx, parentID)
		require.NoError(t, perr, "createWorktree: read parent workspace")
		parentBranch = parent.Branch
	}
	ownWorktree := true
	ws, err := h.app.Usecases.Workspace.CreateChild(ctx, wsusecase.CreateChildInput{
		RepoID:       imported.repoID,
		ProjectID:    imported.projectID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       branch,
		ParentID:     parentID,
		ParentBranch: parentBranch,
		OwnWorktree:  &ownWorktree,
	})
	require.NoErrorf(t, err, "createWorktree: create %q under %q", branch, parentID)

	// The create's writes are asynx commands; the store/list read model is an
	// INDEPENDENT projection that settles out of band. Without this barrier the
	// fixture hands back an id the list may not know — which showed up as
	// "imported workspace missing from list", or, when it is the DAEMON reading
	// the list (DeleteCascade indexes it by id and returns ErrNotFound), as a
	// broadcast that never carries status "deleted" at all. macOS wins this race;
	// Linux loses it roughly 7 times in 10.
	h.Quiesce()

	return ws.ID, owningChatID(t, h, ws.ID)
}

// createChildWorkspace cuts a worktree on branch under parentID and returns its
// workspace id. It is createWorktree for the callers that only want the id;
// resolve the chat with owningChatID when a chat-scoped route is needed.
func createChildWorkspace(
	t *testing.T,
	h *harness,
	imported importedRepo,
	branch string,
	parentID string,
) string {
	t.Helper()
	wsID, _ := createWorktree(t, h, imported, branch, parentID)
	return wsID
}

// worktreeOf returns one workspace's state in the shape the deleted
// GET .../workspaces/:wsId used to answer, read off the chat list — the surface
// that carries it now.
func worktreeOf(
	t *testing.T,
	h *harness,
	imported importedRepo,
	wsID string,
) workspaceDTO {
	t.Helper()
	for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		if w.ID == wsID {
			return w
		}
	}
	require.Failf(t, "workspace not found", "no chat row holds workspace %s", wsID)
	return workspaceDTO{}
}

// repoChatsWS dials the repo-scoped chat feed — the replacement for the deleted
// repo-scoped `workspaces` stream. Frames are dto.AgentChatEvent; read them with
// readUntilWorktree so a predicate written in workspace vocabulary still works.
func repoChatsWS(
	h *harness,
	imported importedRepo,
) *websocket.Conn {
	return h.dial("/v0/projects/" + imported.projectID + "/repos/" + imported.repoID + "/chats/ws")
}

// owningChatID resolves the chat that OWNS wsID, through the daemon's own
// resolver rather than a second one derived here.
//
// EnsureOwningChat runs first — the live-path narrowing of the boot backfill,
// taking the same decision by the same code — because a workspace created
// mid-run is otherwise owed its chat row only by the NEXT boot's backfill, and a
// test that just created one would find nothing to address it by.
func owningChatID(
	t *testing.T,
	h *harness,
	wsID string,
) string {
	t.Helper()
	ctx := context.Background()
	ws, err := h.app.Repositories.Workspace.Get(ctx, wsID)
	require.NoError(t, err, "owningChatID: read workspace %s", wsID)
	require.NoError(
		t,
		h.app.Usecases.AgentChatFolder.EnsureOwningChat(ctx, ws),
		"owningChatID: ensure the owning chat for %s",
		wsID,
	)
	h.Quiesce()
	rows, err := h.app.Usecases.AgentChat.ListChatsByWorkspace(ctx, wsID)
	require.NoError(t, err, "owningChatID: list the chats holding %s", wsID)
	owner, ok := agentusecase.ResolveOwningChat(rows)
	require.Truef(t, ok, "workspace %s must be held by an owning chat", wsID)
	return owner.ID
}

type repoDTO struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Path      string `json:"path"`
}

func listRepos(
	t *testing.T,
	h *harness,
	projectID string,
) []repoDTO {
	t.Helper()
	var repos []repoDTO
	h.get("/v0/projects/"+projectID+"/repos", &repos)
	return repos
}

// workspaceDTO mirrors the migrated WorkspaceDTO wire shape (spec §5): the
// Locked bool is gone (status-based now); ParentID, LastError, CanMergeLocally,
// ParentBranch, and the PR* fields are surfaced.
type workspaceDTO struct {
	ID              string `json:"id"`
	RepoID          string `json:"repoId"`
	ProjectID       string `json:"projectId"`
	Branch          string `json:"branch"`
	ParentID        string `json:"parentId"`
	ForkPointSha    string `json:"forkPointSha"`
	IsDefault       bool   `json:"isDefault"`
	Status          string `json:"status"`
	Working         bool   `json:"working"`
	LastError       string `json:"lastError"`
	Added           int    `json:"added"`
	Deleted         int    `json:"deleted"`
	MergeStrategy   string `json:"mergeStrategy"`
	CanMergeLocally bool   `json:"canMergeLocally"`
	ParentBranch    string `json:"parentBranch"`
	PRUrl           string `json:"prUrl"`
	PRTitle         string `json:"prTitle"`
	PRTargetBranch  string `json:"prTargetBranch"`
	LocalPath       string `json:"localPath"`
	HeldByPath      string `json:"heldByPath"`
	OwningChatID    string `json:"owningChatId"`
}

// listWorkspaces returns the repo's worktree-owning rows in the shape the
// deleted workspace list used to serve.
//
// It reads the CHAT list, which is where that answer lives now: a
// worktree-owning chat carries the workspace's git state inline, projected by
// dto.ChatWorktreeFrom from the very same WorkspaceDTO the old route built
// (spec §5). Rows owning no worktree — bubbles, folders — are not workspaces
// and are skipped. OwningChatID is taken from the chat row itself rather than
// the nested copy, so the id a caller addresses a route with is the id the list
// says the row is.
func listWorkspaces(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
) []workspaceDTO {
	t.Helper()
	var workspaces []workspaceDTO
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree == nil || c.WorkspaceID == "" {
			continue
		}
		w := c.Worktree
		workspaces = append(workspaces, workspaceDTO{
			ID:              c.WorkspaceID,
			RepoID:          repoID,
			ProjectID:       projectID,
			Branch:          w.Branch,
			ParentID:        w.ParentID,
			ForkPointSha:    w.ForkPointSha,
			IsDefault:       w.IsDefault,
			Status:          w.Status,
			Working:         w.Working,
			LastError:       w.LastError,
			Added:           w.Added,
			Deleted:         w.Deleted,
			MergeStrategy:   w.MergeStrategy,
			CanMergeLocally: w.CanMergeLocally,
			ParentBranch:    w.ParentBranch,
			PRUrl:           w.PRUrl,
			PRTitle:         w.PRTitle,
			PRTargetBranch:  w.PRTargetBranch,
			LocalPath:       w.LocalPath,
			HeldByPath:      w.HeldByPath,
			OwningChatID:    c.ID,
		})
	}
	return workspaces
}

// wsBase returns the route prefix a worktree's SHARED leaves hang off —
// git, files, review, search, identity, lsp, terminals — which is the flat
// /v0/chats/:chatId now that spec §8 step 6 deleted the `/workspaces/:wsId`
// group. The chat names the worktree, so this is the same worktree the old
// prefix addressed, reached by the id it is addressable by.
func wsBase(
	imported importedRepo,
) string {
	return "/v0/chats/" + imported.chatID
}

// threadsBase returns the WORKSPACE-scoped prefix review threads still hang off.
//
// Threads is the ONE surviving "/workspaces/:wsId/..." path: it is repo-level
// review commentary, not worktree-owned state, so spec §8 step 6 left it exactly
// where it was while every other leaf moved to the chat prefix. It therefore
// needs its own base — reaching it through wsBase (which is the CHAT prefix now)
// produces a 404, which is precisely the trap this helper exists to remove.
func threadsBase(
	imported importedRepo,
) string {
	return "/v0/projects/" + imported.projectID +
		"/repos/" + imported.repoID +
		"/workspaces/" + imported.workspaceID
}

// chatVerbBase returns the repo-scoped prefix the worktree LIFECYCLE VERBS hang
// off (lock, sync, merge-into-parent, reparent, rebase-onto-parent,
// retry-provision, detach-holder, PATCH branch) plus the chat row itself. Unlike
// the shared leaves above these did NOT go flat: they stayed under the repo
// group, keyed by chat id (endpoints/worktree/routes.go).
func chatVerbBase(
	imported importedRepo,
) string {
	return "/v0/projects/" + imported.projectID +
		"/repos/" + imported.repoID +
		"/chats/" + imported.chatID
}

func writeFile(
	dir string,
	name string,
	content string,
) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
}
