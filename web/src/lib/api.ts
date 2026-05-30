import type { WorkspacePayload, FlowDefinition, ChatMessage, Project } from './types'

const crowbar = (window as any).__CROWBAR__
export const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, init)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}

export function fetchWorkspace(wsId: string): Promise<WorkspacePayload> {
  return apiFetch(`/api/v0/workspaces/${wsId}`)
}

export function postWorkspace(repoId: string, branch: string, flowName: string): Promise<WorkspacePayload> {
  return apiFetch('/api/v0/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repoId, branch, flowName }),
  })
}

export function fetchFlows(): Promise<FlowDefinition[]> {
  return apiFetch('/api/v0/flows')
}

export function fetchConversation(wsId: string, step: string): Promise<{ messages: ChatMessage[] }> {
  return apiFetch(`/api/v0/conversations/${wsId}/${step}`)
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
