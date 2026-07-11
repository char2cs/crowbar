import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'

// Workspace-scoped agentic-chat REST client. Routes nest under
// workspaceBase(wsId)/agent (00 agentic-engine spec §2); the {success,data}
// envelope is unwrapped by apiFetch. Modelled on features/git/api/review-api.ts.

function agentBase(wsId: string): string {
  return `${workspaceBase(wsId)}/agent`
}

// ── Wire shapes (identical to the backend DTOs; camelCase) ──────────
export type AgentSegmentStatus = 'active' | 'ended'

export interface AgentSegment {
  id: string
  providerId: string
  providerSessionId?: string
  crowbarSegmentId: string
  terminalSessionId: string
  startedAt: string
  endedAt?: string
  status: AgentSegmentStatus
}

export interface AgentChat {
  id: string
  workspaceId: string
  title: string
  activeSegmentId: string
  activeProviderId: string
  createdAt: string
}

export interface AgentChatDetail extends AgentChat {
  segments: AgentSegment[]
}

export interface AgentProvider {
  id: string
  displayName: string
  icon: string
}

// ── Mappers (wire → store types). Identity today, but kept explicit so a
//    future wire/store divergence changes one place (review-api idiom). ──
function mapChat(c: AgentChat): AgentChat {
  return {
    id: c.id,
    workspaceId: c.workspaceId,
    title: c.title,
    activeSegmentId: c.activeSegmentId,
    activeProviderId: c.activeProviderId,
    createdAt: c.createdAt,
  }
}

// ── Reads ───────────────────────────────────────────────────────────
export async function listChats(wsId: string): Promise<AgentChat[]> {
  const raw = await apiFetch<AgentChat[]>(`${agentBase(wsId)}/chats`)
  return (raw ?? []).map(mapChat)
}

export async function getChat(wsId: string, id: string): Promise<AgentChatDetail> {
  const raw = await apiFetch<AgentChatDetail>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}`)
  return { ...mapChat(raw), segments: raw.segments ?? [] }
}

export async function listProviders(wsId: string): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(`${agentBase(wsId)}/providers`)
  return raw ?? []
}

// ── Writes ──────────────────────────────────────────────────────────
export async function createChat(wsId: string, provider: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${agentBase(wsId)}/chats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider }),
  })
  return res.id
}

export async function switchProvider(wsId: string, id: string, provider: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(
    `${agentBase(wsId)}/chats/${encodeURIComponent(id)}/switch`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider }),
    },
  )
  return res.id
}

export async function renameChat(wsId: string, id: string, title: string): Promise<void> {
  await apiFetch<unknown>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}/rename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

export async function deleteChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${agentBase(wsId)}/chats/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}
