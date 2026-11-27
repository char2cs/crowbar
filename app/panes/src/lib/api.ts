import type { PRLink } from '@/lib/import/parent-plan'
import { apiFetch, apiUrl } from '@/lib/api/fetch'
import { projectBase, repoBase } from '@/lib/project-url'
import { worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { OperationReply } from '@/core/entities/run-operation'

export function assetURL(path: string): string {
  return apiUrl(path)
}

export const DISCARD_WORK_INIT: RequestInit = { headers: { 'Crowbar-Discard-Work': 'true' } }

export async function renameRepo(projectId: string, repoId: string, name: string): Promise<void> {
  return apiFetch(repoBase(projectId, repoId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

export async function renameProject(projectId: string, name: string): Promise<void> {
  return apiFetch(projectBase(projectId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

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

export function setWorkspaceLock(wsId: string, locked: boolean | null): Promise<void> {
  return apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/lock`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ locked }),
  })
}

export async function getRepoPullRequests(projectId: string, repoId: string): Promise<PRLink[]> {
  return apiFetch<PRLink[]>(`${repoBase(projectId, repoId)}/pull-requests`)
}

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
