import type { WorkspacePayload, Project } from './types'
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
  // Success with an empty/204 body (e.g. WriteMutationOK with no payload, or
  // a 204 No Content): the envelope check below would wrongly throw, so treat
  // it as success returning undefined.
  if (res.ok && (res.status === 204 || body === null)) {
    return undefined as T
  }
  if (!res.ok || !body?.success) {
    throw new ApiError(body?.error ?? `${res.status} ${res.statusText}`, res.status)
  }
  return body.data as T
}

export function fetchWorkspace(wsId: string): Promise<WorkspacePayload> {
  return apiFetch(`/v0/workspaces/${wsId}`)
}

// The backend's WriteMutationOK returns only `{ id }`, not the full entity.
// parentId omitted/empty = fork from the repo's default branch.
export function postWorkspace(
  repoId: string,
  branch: string,
  parentId?: string,
): Promise<{ id: string }> {
  return apiFetch('/v0/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repoId, branch, ...(parentId ? { parentId } : {}) }),
  })
}

export function deleteWorkspace(wsId: string): Promise<void> {
  return apiFetch(`/v0/workspaces/${wsId}`, { method: 'DELETE' })
}

export function fetchProjects(): Promise<Project[]> {
  return apiFetch('/v0/projects')
}

export function fetchProject(id: string): Promise<Project> {
  return apiFetch(`/v0/projects/${id}`)
}

// Pick a real workspace to land on at app start. Prefer the first unlocked
// (editable) workspace so editing works out of the box; fall back to the first
// workspace of any kind, or null when the backend has none yet (→ projects).
export async function fetchLandingWorkspaceId(): Promise<string | null> {
  const workspaces = await apiFetch<Array<{ id: string; locked: boolean }>>('/v0/workspaces')
  if (workspaces.length === 0) return null
  const editable = workspaces.find((ws) => !ws.locked)
  return (editable ?? workspaces[0]).id
}

// The backend's WriteMutationOK returns only `{ id }`, not the full entity.
export function postProject(name: string, path: string): Promise<{ id: string }> {
  return apiFetch('/v0/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}
