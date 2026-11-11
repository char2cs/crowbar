import type { Prerequisites, RepoDTO, WorkspaceDTO } from './types'
import type { PRLink } from '@/lib/import/parent-plan'
import { apiFetch, apiUrl } from '@/lib/api/fetch'
import { projectBase, repoBase } from '@/lib/project-url'
import { worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'
// Type-only: keeps this module out of the entity store's import graph.
import type { OperationReply } from '@/core/entities/run-operation'

// A daemon-relative asset path the webview can load, for URLs handed to the browser (`<img src>`,
// not apiFetch). A bare `/v0/...` path would resolve to Vite or the app bundle and 404 into the
// default mark; one function so the repo avatar and project icon resolve alike.
export function assetURL(path: string): string {
  return apiUrl(path)
}

/** The request options that consent to a delete destroying work at risk. */
export const DISCARD_WORK_INIT: RequestInit = { headers: { 'Crowbar-Discard-Work': 'true' } }

// ---------------------------------------------------------------------------
// Hierarchical READ API (§3/§7).
// ---------------------------------------------------------------------------

// Project-scoped calls are async: an unresolved host rejects like a failed request, never throws.
export async function fetchRepos(projectId: string): Promise<RepoDTO[]> {
  return apiFetch(`${projectBase(projectId)}/repos`)
}

// A repo's workspaces from the GET .../workspaces resource, never derived from chats: a workspace
// with no chat (a default checkout nobody chatted in, an unopened locked branch) would vanish, and
// with the default one the repo header too. Live worktree frames only refresh known workspaces.
export async function fetchWorkspaces(projectId: string, repoId: string): Promise<WorkspaceDTO[]> {
  const rows = await apiFetch<WorkspaceDTO[]>(`${repoBase(projectId, repoId)}/workspaces`)
  return rows ?? []
}

// ---------------------------------------------------------------------------
// Hierarchical WRITE API (§3/§7). Creates and deletes answer 202 with an operation (whenSettled).
// ---------------------------------------------------------------------------

/** Create a project on `hostId`: it is not in the project list yet, so no row can name its host. */
export function postProject(
  name: string,
  path: string,
  opId: string,
  hostId: string,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(
    '/v0/projects',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Idempotency-Key': opId },
      body: JSON.stringify({ name, path }),
    },
    undefined,
    hostId,
  )
}

export async function postRepo(
  projectId: string,
  name: string,
  path: string,
  opId: string,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(`${projectBase(projectId)}/repos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': opId },
    body: JSON.stringify({ name, path }),
  })
}

// Rename an already-added repo. Answers 204; the updated RepoDTO (new name +
// regenerated avatar) arrives on the repos WS stream and merges into the cache,
// so callers do not update the sidebar store themselves.
export async function renameRepo(projectId: string, repoId: string, name: string): Promise<void> {
  return apiFetch(repoBase(projectId, repoId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

// Rename a project; the renamed ProjectDTO arrives on the projects stream, as with renameRepo.
export async function renameProject(projectId: string, name: string): Promise<void> {
  return apiFetch(projectBase(projectId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

/** Delete a project, and with it every repo and workspace inside. */
export async function deleteProject(
  projectId: string,
  opId: string,
  init?: RequestInit,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(projectBase(projectId), {
    method: 'DELETE',
    ...init,
    headers: { ...init?.headers, 'Idempotency-Key': opId },
  })
}

// Set or clear a workspace's lock. `null` is a third state, not false: it drops the user's
// override so the branch is locked exactly when it is protected. Automatic locking is unaffected.
export function setWorkspaceLock(wsId: string, locked: boolean | null): Promise<void> {
  return apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/lock`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ locked }),
  })
}

// Open-PR head→base graph for the import dialog's parent hint. Advisory only:
// the import endpoint re-resolves parenting authoritatively server-side.
export async function getRepoPullRequests(projectId: string, repoId: string): Promise<PRLink[]> {
  return apiFetch<PRLink[]>(`${repoBase(projectId, repoId)}/pull-requests`)
}

// Batch-import branches as chats holding a worktree each; answers 202 with one operation item per
// branch (named by its caller-minted chat id); `lock` creates it locked. Not a loop over POST
// .../chats: only this resolves PR parents across the set, creates ancestors, and placeholders.
export async function importBranches(
  projectId: string,
  repoId: string,
  branches: { name: string; chatId: string; lock: boolean }[],
  opId: string,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(`${repoBase(projectId, repoId)}/chats/import-batch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Idempotency-Key': opId },
    body: JSON.stringify({ branches }),
  })
}

// Remove a repository, worktrees and all; the RepoDTO and tombstones arrive on the entity streams.
// The one removal the sidebar asks the user to confirm.
export async function deleteRepo(
  projectId: string,
  repoId: string,
  opId: string,
  init?: RequestInit,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(repoBase(projectId, repoId), {
    method: 'DELETE',
    ...init,
    headers: { ...init?.headers, 'Idempotency-Key': opId },
  })
}

// Rename a worktree's branch through its owning chat, like lock and merge. Only the git ref changes
// (the directory stays, so cwds survive); the worktree arrives on the chat stream. A refusal is a
// readable 409 while the editor is up. The URL comes from the recorded scope; ids are unused.
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

/** One tool's check, so each row shows as soon as its probe answers. */
export function fetchPrerequisite<T extends keyof Prerequisites>(
  tool: T,
): Promise<Prerequisites[T]> {
  return apiFetch(`/v0/system/prerequisites/${tool}`)
}
