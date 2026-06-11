import { apiFetch } from '@/lib/api'

/**
 * Agent run as returned by the backend (domain.AgentRun). A run is the
 * lifecycle record behind a chat send: create → start → complete|fail.
 * Starting a run flips the chat to `agent-running` (and the workspace's
 * agentRunning overlay to true) via the /v0/ws/chats broadcaster.
 */
export interface AgentRunDto {
  id: string
  wsId: string
  chatId: string
  status: 'queued' | 'running' | 'done' | 'error'
  createdAt: string
}

// POST /v0/workspaces/:wsId/runs → 201, data = full run object (status queued).
export function postRun(wsId: string, chatId: string): Promise<AgentRunDto> {
  return apiFetch(`/v0/workspaces/${encodeURIComponent(wsId)}/runs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ chatId }),
  })
}

// POST /v0/runs/:id/start → 200, data = run with status running.
export function startRun(id: string): Promise<AgentRunDto> {
  return apiFetch(`/v0/runs/${encodeURIComponent(id)}/start`, { method: 'POST' })
}

// POST /v0/runs/:id/complete → 200, data = run with status done.
export function completeRun(id: string): Promise<AgentRunDto> {
  return apiFetch(`/v0/runs/${encodeURIComponent(id)}/complete`, { method: 'POST' })
}

// POST /v0/runs/:id/fail → 200, data = run with status error.
export function failRun(id: string): Promise<AgentRunDto> {
  return apiFetch(`/v0/runs/${encodeURIComponent(id)}/fail`, { method: 'POST' })
}
