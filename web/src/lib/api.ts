import type { WorkspacePayload, Project } from './types'
import { useChaosStore } from '@/lib/store/chaos'

const crowbar = (window as any).__CROWBAR__
export const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

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
    throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
  }
  return body.data as T
}

export function fetchWorkspace(wsId: string): Promise<WorkspacePayload> {
  return apiFetch(`/v0/workspaces/${wsId}`)
}

// The backend's WriteMutationOK returns only `{ id }`, not the full entity.
export function postWorkspace(repoId: string, branch: string): Promise<{ id: string }> {
  return apiFetch('/v0/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repoId, branch }),
  })
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
  const workspaces = await apiFetch<Array<{ id: string; locked: boolean }>>(
    '/v0/workspaces',
  )
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
