import type { Project, Prerequisites, RepoDTO, WorkspaceDTO } from './types'
import { useChaosStore } from '@/lib/store/chaos'

const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__
export const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

/** Error thrown by apiFetch carrying the HTTP status, so callers can make
 *  status-specific decisions (e.g. a 404 is terminal — never retried). */
export class ApiError extends Error {
  readonly status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export function isNotFoundError(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const { latency, errorRate, scenario, faults } = useChaosStore.getState()
  const chaosHeaders: Record<string, string> = {}
  if (latency > 0) chaosHeaders['X-Crowbar-Latency'] = String(latency)
  if (errorRate > 0) chaosHeaders['X-Crowbar-Error-Rate'] = String(errorRate)

  if (import.meta.env.VITE_USE_MOCK === 'true') {
    chaosHeaders['X-Crowbar-Scenario'] = scenario
    const activeFaults = Object.entries(faults).filter(([, v]) => v > 0)
    if (activeFaults.length > 0) {
      chaosHeaders['X-Crowbar-Fault'] = JSON.stringify(Object.fromEntries(activeFaults))
    }
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { ...init?.headers, ...chaosHeaders },
  })
  const body = await res.json().catch(() => null)
  // Success with an empty/204/202 body (e.g. WriteMutationOK with no payload, a
  // 204 No Content, or a 202 Accepted for an async hierarchical mutation): the
  // envelope check below would wrongly throw, so treat it as success returning
  // undefined.
  if (res.ok && (res.status === 204 || res.status === 202 || body === null)) {
    return undefined as T
  }
  if (!res.ok || !body?.success) {
    throw new ApiError(body?.error ?? `${res.status} ${res.statusText}`, res.status)
  }
  return body.data as T
}

export function fetchProjects(): Promise<Project[]> {
  return apiFetch('/v0/projects')
}

export function fetchProject(id: string): Promise<Project> {
  return apiFetch(`/v0/projects/${id}`)
}

// ---------------------------------------------------------------------------
// Hierarchical READ API (§3/§7).
// ---------------------------------------------------------------------------

export function fetchRepos(projectId: string): Promise<RepoDTO[]> {
  return apiFetch(`/v0/projects/${projectId}/repos`)
}

export function fetchWorkspaces(projectId: string, repoId: string): Promise<WorkspaceDTO[]> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces`)
}

export function fetchWorkspace(
  projectId: string,
  repoId: string,
  wsId: string,
): Promise<WorkspaceDTO> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces/${wsId}`)
}

// ---------------------------------------------------------------------------
// Hierarchical WRITE API (§3/§7) — every mutation is fire-and-forget: the
// daemon answers 202 Accepted with an empty body and the real entity (with its
// status transitions) arrives over the scoped WS broadcaster. Callers therefore
// await the WS DTO for navigation, never an id from these calls.
// ---------------------------------------------------------------------------

export function postProject(name: string, path: string, quick?: boolean): Promise<void> {
  return apiFetch('/v0/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path, ...(quick ? { quick: true } : {}) }),
  })
}

export function postRepo(projectId: string, name: string, path: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}

// parentId omitted/empty = fork from the repo's default branch.
export function postWorkspace(
  projectId: string,
  repoId: string,
  branch: string,
  parentId?: string,
): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branch, ...(parentId ? { parentId } : {}) }),
  })
}

export function deleteWorkspace(projectId: string, repoId: string, wsId: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/workspaces/${wsId}`, {
    method: 'DELETE',
  })
}

export function fetchPrerequisites(): Promise<Prerequisites> {
  return apiFetch('/v0/system/prerequisites')
}
