import type { ChatDTO, FolderDTO, Project, Prerequisites, RepoDTO, WorkspaceDTO } from './types'
import type { PRLink } from '@/lib/import/parent-plan'
import { getOwningChatId } from '@/lib/workspace-scope'
import { API_BASE, apiFetch } from '@/lib/api/fetch'
import {
  chatDTOFromWire,
  folderDTOFromWire,
  workspaceDTOFromChat,
  type ChatsFolderWireDTO,
  type RepoChatWireDTO,
} from '@/lib/api/wire'
import { OwningChatNotRecordedError, worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'
// Type-only: the store module reaches back into this one through the sidebar stores.
import type { OperationReply } from '@/lib/store/run-operation'

/**
 * Turn a daemon-relative asset path into one the webview can actually load.
 *
 * Only for URLs handed to the BROWSER — an `<img src>`, not an `apiFetch` (which
 * applies API_BASE itself). A DTO's icon URL arrives as a bare `/v0/...` path,
 * and the desktop webview is served from its own origin: on the dev server that
 * path resolves to Vite, in a packaged build to the app bundle. Either way it is
 * not the daemon, so the request 404s and the <img> quietly falls back to the
 * entity's default mark — a broken icon that looks exactly like an icon nobody
 * set.
 *
 * One function, so the repo avatar and the project icon cannot
 * resolve the same kind of URL two different ways.
 */
export function assetURL(path: string): string {
  return `${API_BASE}${path}`
}

/** The request options that consent to a delete destroying work at risk. */
export const DISCARD_WORK_INIT: RequestInit = { headers: { 'Crowbar-Discard-Work': 'true' } }

export function fetchProjects(): Promise<Project[]> {
  return apiFetch('/v0/projects')
}

// ---------------------------------------------------------------------------
// Hierarchical READ API (§3/§7).
// ---------------------------------------------------------------------------

export function fetchRepos(projectId: string): Promise<RepoDTO[]> {
  return apiFetch(`/v0/projects/${projectId}/repos`)
}

/**
 * One repo's workspaces, straight off the real GET .../workspaces resource.
 *
 * This USED to derive WorkspaceDTOs from the chat list (a worktree rides each
 * chat row as `worktree`) on the theory that there was no workspace resource
 * left to list — every worktree is held by SOME chat, so deriving from chats
 * was "just" a join. That theory breaks for a workspace with NO chat at all: a
 * repo's own default checkout before anyone has chatted in it, or any locked
 * tracking branch nobody ever opened a conversation in. Neither has a chat row
 * to derive from, so the old join silently produced zero rows for them — not
 * just missing branches, but a missing REPO HEADER (rows-from-repo.ts mints
 * that from the default workspace) — reproduced live against real production
 * data: an entire repo, and a repo's own other locked branches, absent from
 * the sidebar.
 *
 * The live half (an open chat's `worktree_state` frames, `workspaceDTOFromChat`
 * in wire.ts) is unaffected — it only ever refreshes a workspace already in the
 * store, never discovers one — so it is untouched.
 */
export async function fetchWorkspaces(projectId: string, repoId: string): Promise<WorkspaceDTO[]> {
  const rows = await apiFetch<WorkspaceDTO[]>(
    `/v0/projects/${projectId}/repos/${repoId}/workspaces`,
  )
  return rows ?? []
}

/**
 * One repo's sidebar folders, in sidebar order.
 *
 * There is no dedicated push channel for this any more — folders lost their
 * own REST+WS resource (backend plan closed; see app-sync-provider.tsx's
 * folders subscription for how a change now reaches the sidebar tree).
 *
 * KNOWN BACKEND LIMITATION, not fixed here: the daemon's `ListInRepo`
 * (api/internal/app/usecases/chat/internal/tree/tree.go) never actually
 * filters by its own `repoID` argument — it filters only by
 * `Type == folder`, so `GET .../chats/folders` returns EVERY folder in the
 * whole daemon, not just this repo's (already self-disclosed in the
 * backend's own container.go comment on the folder push frame's repo
 * scoping). `folderDTOFromWire` below stamps every row it gets back with
 * THIS call's own `repoId`/`projectId` regardless of which repo the row
 * really belongs to, since the wire carries neither — so with more than one
 * repo open, each repo's folder list can end up claiming another repo's
 * folders as its own. There is no correct frontend workaround (the wire row
 * carries no real repoId to filter on); this needs a Go-side fix, out of
 * scope for this task and the closed backend plan.
 */
export async function fetchFolders(projectId: string, repoId: string): Promise<FolderDTO[]> {
  const rows = await apiFetch<ChatsFolderWireDTO[]>(
    `/v0/projects/${projectId}/repos/${repoId}/chats/folders`,
  )
  return (rows ?? []).map((row) => folderDTOFromWire(row, projectId, repoId))
}

/**
 * One repo's chat rows, in sidebar order.
 *
 * Genuinely repo-scoped, unlike `fetchFolders` above: the daemon's
 * `ListChatsInRepo` (api/internal/app/usecases/chat/repo_scope.go) resolves each
 * row's owning repo by walking its ancestry to the nearest provisioned workspace
 * and keeps only the rows that land in THIS repo. A row whose whole ancestry owns
 * no workspace resolves to no repo and is served to none — so a root bubble the
 * spec's §9.1 open question flags as unplaceable never arrives here at all,
 * rather than arriving in every repo's list at once.
 *
 * Stamping this call's own repoId/projectId onto the rows is therefore honest
 * here, where the same line in `fetchFolders` is the documented cross-repo bleed:
 * the server already guaranteed every row belongs to the repo in the URL.
 *
 * Live updates ride `folder-signal.ts`'s per-repo generation, which
 * `use-workspace-agent-chats-stream.ts` bumps on the structural chat frames
 * (created / deleted / title_set / moved) as well as the folder ones — one
 * signal, because chats and folders are ONE aggregate (`domain.Chat`) and one
 * tree.
 */
export async function fetchRepoChats(projectId: string, repoId: string): Promise<ChatDTO[]> {
  const rows = await apiFetch<RepoChatWireDTO[]>(`/v0/projects/${projectId}/repos/${repoId}/chats`)
  return (rows ?? []).map((row) => chatDTOFromWire(row, projectId, repoId))
}

/**
 * One workspace, read through the CHAT that owns its worktree.
 *
 * Callers still hold only a wsId, so the owning chat is resolved from the scope
 * registry the sidebar records — and a missing one throws rather than guessing a
 * URL, the same contract `worktreeVerbBaseForWorkspace` keeps.
 */
export async function fetchWorkspace(
  projectId: string,
  repoId: string,
  wsId: string,
): Promise<WorkspaceDTO> {
  const chatId = getOwningChatId(wsId)
  if (!chatId) throw new OwningChatNotRecordedError(wsId)
  const row = await apiFetch<RepoChatWireDTO>(
    `/v0/projects/${projectId}/repos/${repoId}/chats/${chatId}`,
  )
  const workspace = row ? workspaceDTOFromChat(row, projectId, repoId) : null
  if (!workspace) throw new Error(`chat ${chatId} holds no worktree for workspace ${wsId}`)
  return workspace
}

// The PROJECT-level home workspace, which is a different entity from a repo's
// default branch: it rides no repo at all, so it appears in NO repo's chat list
// and cannot be derived from one. This route stays as it is.
export function fetchHomeWorkspace(projectId: string): Promise<WorkspaceDTO> {
  return apiFetch(`/v0/projects/${projectId}/home`)
}

/**
 * The project-home workspace's own chat rows and folders, in sidebar order.
 *
 * Same wire shapes as a repo's `/chats` and `/chats/folders` (the daemon
 * mounts the SAME `chathandlers.Handlers` at both routes — see
 * `api/internal/api/v0/endpoints/home/routes.go`), just with no `repoId` to
 * stamp: `''` is the sentinel every home-scoped `ChatDTO`/`FolderDTO` carries,
 * matching `WorkspaceDTO.repoId` for the home workspace itself.
 */
export async function fetchHomeChats(projectId: string): Promise<ChatDTO[]> {
  const rows = await apiFetch<RepoChatWireDTO[]>(`/v0/projects/${projectId}/home/chats`)
  return (rows ?? []).map((row) => chatDTOFromWire(row, projectId, ''))
}

export async function fetchHomeFolders(projectId: string): Promise<FolderDTO[]> {
  const rows = await apiFetch<ChatsFolderWireDTO[]>(`/v0/projects/${projectId}/home/chats/folders`)
  return (rows ?? []).map((row) => folderDTOFromWire(row, projectId, ''))
}

// ---------------------------------------------------------------------------
// Hierarchical WRITE API (§3/§7). A create or a delete answers 202 with its
// operation under the caller's Idempotency-Key; the entity itself arrives over
// the scoped WS broadcaster, and `whenSettled` waits for both.
// ---------------------------------------------------------------------------

export function postProject(name: string, path: string, opId: string): Promise<OperationReply> {
  return apiFetch<OperationReply>('/v0/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': opId },
    body: JSON.stringify({ name, path }),
  })
}

export function postRepo(
  projectId: string,
  name: string,
  path: string,
  opId: string,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(`/v0/projects/${projectId}/repos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': opId },
    body: JSON.stringify({ name, path }),
  })
}

// Rename an already-added repo. Answers 204; the updated RepoDTO (new name +
// regenerated avatar) arrives on the repos WS stream and merges into the cache,
// so callers do not update the sidebar store themselves.
export function renameRepo(projectId: string, repoId: string, name: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

/**
 * Rename a project. The renamed ProjectDTO arrives on the projects WS stream, so
 * no caller patches its own cache from this — same contract as renameRepo.
 */
export function renameProject(projectId: string, name: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

/** Delete a project, and with it every repo and workspace inside. */
export function deleteProject(
  projectId: string,
  opId: string,
  init?: RequestInit,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(`/v0/projects/${projectId}`, {
    method: 'DELETE',
    ...init,
    headers: { ...init?.headers, 'Idempotency-Key': opId },
  })
}

/**
 * Set (or clear) a workspace's lock.
 *
 * `locked` null is the third state, not a synonym for false: it drops the user's
 * override and hands the question back to the provider, so a branch goes back to
 * being locked exactly when it is protected. Automatic locking is unaffected
 * either way — this only decides whether the user is overruling it.
 */
export function setWorkspaceLock(wsId: string, locked: boolean | null): Promise<void> {
  return apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/lock`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ locked }),
  })
}

// Open-PR head→base graph for the import dialog's parent hint. Advisory only:
// the import endpoint re-resolves parenting authoritatively server-side.
export function getRepoPullRequests(projectId: string, repoId: string): Promise<PRLink[]> {
  return apiFetch<PRLink[]>(`/v0/projects/${projectId}/repos/${repoId}/pull-requests`)
}

// Batch-import branches as chats holding a worktree each. The daemon PR-parents
// each branch under the row for its open PR's base and creates missing ancestors
// (the whole tree). Answers 202 with the operation: one item per branch, named
// by the chat id the caller minted for it, settling with each branch's outcome.
// A branch's `lock` creates its workspace locked.
//
// It is a route of its own rather than a loop over POST .../chats, which adopts
// ONE named branch: only this one resolves the PR graph ACROSS a set, creates
// the ancestors a branch is parented under, and falls back to a placeholder row
// for a branch another worktree already holds. Driving it per-branch would drop
// all three silently.
export function importBranches(
  projectId: string,
  repoId: string,
  branches: { name: string; chatId: string; lock: boolean }[],
  opId: string,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(`/v0/projects/${projectId}/repos/${repoId}/chats/import-batch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': opId },
    body: JSON.stringify({ branches }),
  })
}

// Remove a repository from the project, worktrees and all. The removed RepoDTO
// and its workspaces' tombstones arrive on the entity streams; nothing here
// writes the sidebar tree. This is the one removal the sidebar asks the user to
// confirm — everything under the repo goes with it.
export function deleteRepo(
  projectId: string,
  repoId: string,
  opId: string,
  init?: RequestInit,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(`/v0/projects/${projectId}/repos/${repoId}`, {
    method: 'DELETE',
    ...init,
    headers: { ...init?.headers, 'Idempotency-Key': opId },
  })
}

// Rename a worktree's branch, addressed through the CHAT that holds it — a
// worktree verb like lock and merge, on the same repo-scoped chat prefix. The
// daemon renames the git ref and nothing else: the directory is NOT relocated
// any more, so a chat's cwd survives the rename. The updated worktree arrives on
// the chat stream, so — as with renameRepo — callers do not update the sidebar
// store themselves. Answers synchronously: a refusal (name taken, worktree
// locked, branch adopted, not provisioned) arrives as a 409 with a readable
// message while the inline editor is still on screen.
//
// `projectId`/`repoId` stay on the signature for the callers that hold them;
// the URL comes from the recorded scope, which is the only place the owning
// chat is known.
export function renameWorkspaceBranch(
  _projectId: string,
  _repoId: string,
  wsId: string,
  branch: string,
): Promise<void> {
  return apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/branch`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branch }),
  })
}

export function fetchPrerequisites(): Promise<Prerequisites> {
  return apiFetch('/v0/system/prerequisites')
}
