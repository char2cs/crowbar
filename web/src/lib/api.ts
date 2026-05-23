import type { WorkspacePayload, FlowDefinition, ChatMessage, Project } from './types'
import { getMockWorkspace, createMockWorkspace } from './mock/workspaces'
import { MOCK_FLOWS } from './mock/flows'
import { getMockConversation } from './mock/conversations'
import { getAllMockProjects, createMockProject } from './mock/projects'

const crowbar = (window as any).__CROWBAR__
export const API_BASE = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

export function apiFetch(path: string, init?: RequestInit): Promise<unknown> {
  return fetch(`${API_BASE}${path}`, init).then(r => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`)
    return r.json()
  })
}

export function fetchWorkspace(wsId: string): Promise<WorkspacePayload> {
  const ws = getMockWorkspace(wsId)
  if (!ws) return Promise.reject(new Error(`Unknown workspace: ${wsId}`))
  return Promise.resolve(ws)
}

export function postWorkspace(
  repoId: string,
  branch: string,
  flowName: string,
): Promise<WorkspacePayload> {
  return Promise.resolve(createMockWorkspace(repoId, branch, flowName))
}

export function fetchFlows(): Promise<FlowDefinition[]> {
  return Promise.resolve(MOCK_FLOWS)
}

export function fetchConversation(
  wsId: string,
  step: string,
): Promise<{ messages: ChatMessage[] }> {
  return Promise.resolve({ messages: getMockConversation(wsId, step) })
}

export function fetchProjects(): Promise<Project[]> {
  return Promise.resolve(getAllMockProjects())
}

export function postProject(name: string, path: string): Promise<Project> {
  return Promise.resolve(createMockProject({ name, path }))
}
