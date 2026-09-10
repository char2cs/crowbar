// Package handlers serves the /v0/projects/:projectId/home routes.
package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// WSConn is the WebSocket abstraction used by the terminal engine Attach method.
type WSConn = engineterminal.WSConn

// ProjectReader resolves a project by ID — used for lazy home provisioning.
type ProjectReader interface {
	FindByKey(ctx context.Context, id string) (*domain.Project, error)
}

// HomeWorkspaces is the workspace surface the home handlers need.
type HomeWorkspaces interface {
	GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error)
	CreateHome(ctx context.Context, projectID, worktreePath string, now time.Time) (domain.Workspace, error)
}

// Files is the file usecase surface needed by home file handlers.
type Files interface {
	// Tree returns one level of the file tree for a workspace.
	Tree(
		ctx context.Context,
		wsID string,
		dirPath string,
		provider fileusecase.FileStatusProvider,
	) ([]domain.FileNode, error)

	// ReadContent reads a file in a workspace.
	ReadContent(
		ctx context.Context,
		wsID string,
		filePath string,
	) (domain.FileContent, error)

	// WriteContent writes a file and resyncs the working tree. encoding is
	// "base64" for a byte-faithful binary payload or "" / "utf8" for raw UTF-8.
	WriteContent(
		ctx context.Context,
		wsID string,
		filePath string,
		content string,
		encoding string,
		now time.Time,
	) error

	// CreateFile creates a file and resyncs the working tree.
	CreateFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error

	// CreateDir creates a directory and resyncs the working tree.
	CreateDir(
		ctx context.Context,
		wsID string,
		dirPath string,
		now time.Time,
	) error

	// Copy duplicates a file or directory byte-faithfully and resyncs the
	// working tree.
	Copy(
		ctx context.Context,
		wsID string,
		sourcePath string,
		destPath string,
		now time.Time,
	) error

	// Rename renames a path and resyncs the working tree.
	Rename(
		ctx context.Context,
		wsID string,
		oldPath string,
		newPath string,
		now time.Time,
	) error

	// Delete removes a path and resyncs the working tree.
	Delete(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
}

// TerminalEngine is the terminal engine surface needed by home terminal handlers.
//
// The engine keys sessions by their OWNING CHAT now (spec §4.2). The home
// group has no chat behind it — that is exactly what spec §4.1 deletes it for —
// so it passes its own home workspace id as the key and is merely
// self-consistent: it lists back what it created, under an id no chat shares.
// This whole group, these four terminal routes included, goes away in spec §8
// step 6; nothing new should be built on it.
type TerminalEngine interface {
	Create(
		ctx context.Context,
		ownerID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)
	Kill(
		ctx context.Context,
		sessionID string,
	) error
	ListSessionsForChat(
		ownerID string,
	) []string
	SessionExists(ctx context.Context, sessionID string) bool
	Attach(ctx context.Context, sessionID string, conn WSConn) error
}

// ChatResolver resolves the chat rows a workspace id owns, mirroring the
// workspaces handlers' own ChatResolver (workspacehandlers.ChatResolver): the
// home workspace's GET is a second wire-DTO call site, and needs the same
// read to resolve WorkspaceDTO.OwningChatID.
type ChatResolver interface {
	ListChatsByWorkspace(
		ctx context.Context,
		workspaceID string,
	) ([]domain.Chat, error)
}

// WorkSignal is the read half of the daemon's working overlay, the same seam the
// workspaces handlers stamp their REST reads from (workspacehandlers.WorkSignal,
// which the repositories Container satisfies). The home workspace is served by
// THIS endpoint rather than the workspace-scoped list/detail, so it must stamp
// the overlay itself or its REST read would report working=false while an agent
// chat anchored to the project home is mid-turn — diverging from the very WS
// frames the container enriches for it. Only the read is needed here: the home
// endpoint runs no async mutations, so it never brackets a work window.
type WorkSignal interface {
	// WorkingFor reports whether the workspace is working via EITHER derived
	// overlay (an inflight background mutation OR an agent chat mid-turn).
	WorkingFor(
		wsID string,
	) bool
}

// NodeCreator is the narrow write-only Node surface resolveHome's lazy
// provisioning needs to mint a legacy project's home workspace its own
// position row the instant it creates one (2026-09-08
// sidebar-placement-unification Task 7) — every OTHER home-workspace creation
// path (a fresh project import) already mints one via project.ImportDeps.Nodes;
// this is the one lazy-provisioning path that lives outside that usecase.
//
// CreateIdempotent, not Create: the node's own id here is homeWorkspaceID's
// deterministic one (workspace.go's own doc), which a concurrent caller can
// legitimately race to create too — see node.EventStore.CreateIdempotent's
// own doc for why that specific id shape is what makes tolerating the loss
// safe, rather than masking a real bug.
type NodeCreator interface {
	CreateIdempotent(
		ctx context.Context,
		id string,
		kind domain.NodeKind,
		parentID string,
		order int,
	) (domain.Node, error)
}

// Handlers serves all /home/* routes.
type Handlers struct {
	workspaces HomeWorkspaces
	projects   ProjectReader
	files      Files
	termEng    TerminalEngine
	working    WorkSignal
	chats      ChatResolver
	nodes      NodeCreator
}

// New builds Handlers.
func New(
	workspaces HomeWorkspaces,
	projects ProjectReader,
	files Files,
	termEng TerminalEngine,
	working WorkSignal,
) *Handlers {
	return &Handlers{
		workspaces: workspaces,
		projects:   projects,
		files:      files,
		termEng:    termEng,
		working:    working,
	}
}

// WithChats wires the chat-row resolver Get needs to answer
// WorkspaceDTO.OwningChatID for the home workspace. A nil chats leaves it
// unwired, and Get degrades to an empty owningChatId rather than panicking.
func (h *Handlers) WithChats(
	chats ChatResolver,
) *Handlers {
	if chats != nil {
		h.chats = chats
	}
	return h
}

// WithNodes wires the Node surface the lazy home-provisioning path in
// resolveHome mints a legacy project's home workspace's own position row
// through. Unlike WithChats this is NOT optional in effect: resolveHome
// refuses the request when CreateHome runs but no Nodes surface was wired,
// the same ErrNoNodesWired-style refusal project.ImportDeps.Nodes already
// enforces for every other workspace-creation path (2026-09-08
// sidebar-placement-unification Task 7) — a lazily-provisioned home workspace
// must not persist with no position row any more than a freshly-imported one
// may.
func (h *Handlers) WithNodes(
	nodes NodeCreator,
) *Handlers {
	h.nodes = nodes
	return h
}

// resolveHome fetches the home workspace for the project. If not yet
// provisioned (ErrNotFound), it looks up the project path and creates one
// lazily — supporting projects created before the home feature was introduced.
//
// The provisioning branch used to be a plain read (GetHomeForProject) then a
// write (CreateHome) with nothing serializing the two: two requests landing
// before the first's write was visible both read ErrNotFound and both
// provisioned, and CreateHome minted a fresh random uuid every call with
// nothing to stop it, so the project ended up with TWO home workspaces.
// Nothing after ever noticed: each caller's own response carried whichever
// one IT just made, and the frontend resolver (home-workspace-resolver.ts)
// caches that id for the rest of the session, "a lookup, not a mint" per its
// own doc — a guarantee that race broke. Caught live: a project's home
// stopped minting CLIs entirely, every attempt failing "asynx: aggregate not
// found" — the frontend had cached the LOSING workspace's id, which every
// later GetHomeForProject scan (the winner, whichever id it happens to
// return) never answers again.
//
// Fixed at the root rather than by adding a lock in front of it:
// CreateHome/nodes.CreateIdempotent no longer mint a random id for a
// project's home — they derive it deterministically from the project id
// (workspace.homeWorkspaceID's own doc), so two concurrent provisions for
// the SAME project now contend for the SAME aggregate, which asynx's own
// per-aggregate command serialization already resolves exactly like every
// other write in this system: one commits, the other is refused and reads
// the winner back directly. This handler has nothing left to serialize
// itself — no mutex, no singleflight, nothing process-local that a second
// daemon instance or a retried request years apart would need again.
func (h *Handlers) resolveHome(c *gin.Context) (domain.Workspace, bool) {
	ctx := c.Request.Context()
	projectID := c.Param("projectId")
	ws, err := h.workspaces.GetHomeForProject(ctx, projectID)
	if err == nil {
		return ws, true
	}
	if !errors.Is(err, apperr.ErrNotFound) {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to resolve home workspace")
		return domain.Workspace{}, false
	}

	// Lazily provision: look up the project to get its path, then create.
	project, pErr := h.projects.FindByKey(ctx, projectID)
	if pErr != nil || project == nil {
		libs.WriteErr(c, http.StatusNotFound, "project not found")
		return domain.Workspace{}, false
	}
	ws, cErr := h.workspaces.CreateHome(ctx, projectID, project.Path, time.Now())
	if cErr != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to provision home workspace")
		return domain.Workspace{}, false
	}
	// The freshly-provisioned home workspace mints its OWN Node{Kind:workspace}
	// row unconditionally, right here at creation — mirrors every other
	// workspace-creation path (project.createOwnedWorkspace,
	// hierarchy.CreateChild/adoptMainWorktree/importPlaceholder). CreateHome
	// already resolved the "who actually gets to provision" race for ws
	// itself; this node row rides the SAME id, so it needs the SAME
	// tolerance for losing to a concurrent winner — CreateIdempotent, not
	// Create.
	if h.nodes == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "no node creator wired")
		return domain.Workspace{}, false
	}
	if _, nErr := h.nodes.CreateIdempotent(ctx, ws.ID, domain.NodeKindWorkspace, "", 0); nErr != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to provision home workspace position")
		return domain.Workspace{}, false
	}
	return ws, true
}

// RequireHomeWorkspace resolves the project's home workspace and injects its id
// as the :wsId path param, so handlers and the WS broadcaster reused from the
// repo-scoped surface (the file-change WS, review threads) resolve the home
// workspace by id without a dedicated home implementation. The home workspace
// has no repo, so :repoId stays empty — the thread namespace, filters, and
// snapshot all tolerate the empty middle segment (clientScope trims only
// trailing empties, so "p//w" still prefix-matches "p//w/<id>"). On failure
// resolveHome has already written the error envelope; we just abort the chain.
func (h *Handlers) RequireHomeWorkspace(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		c.Abort()
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "wsId", Value: ws.ID})
	c.Next()
}
