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
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}

export function fetchWorkspace(wsId: string): Promise<WorkspacePayload> {
  return apiFetch(`/api/v0/workspaces/${wsId}`)
}

export function postWorkspace(repoId: string, branch: string): Promise<WorkspacePayload> {
  return apiFetch('/api/v0/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repoId, branch }),
  })
}


export function fetchProjects(): Promise<Project[]> {
  return apiFetch('/api/v0/projects')
}

export function postProject(name: string, path: string): Promise<Project> {
  return apiFetch('/api/v0/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}
