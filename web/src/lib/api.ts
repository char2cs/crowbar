import type { Prerequisites, RepoDTO, WorkspaceDTO } from './types'
import type { PRLink } from '@/lib/import/parent-plan'
import { API_BASE, apiFetch } from '@/lib/api/fetch'
import { worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'
// Type-only: keeps this module out of the entity store's import graph.
import type { OperationReply } from '@/core/entities/run-operation'

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
 * just missing branches, but a missing REPO HEADER (deriveRows mints
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

// ---------------------------------------------------------------------------
// Hierarchical WRITE API (§3/§7). A create or a delete answers 202 with its
// operation under the caller's Idempotency-Key; the entity itself arrives over
// the projects scope, and `whenSettled` waits for the operation.
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
