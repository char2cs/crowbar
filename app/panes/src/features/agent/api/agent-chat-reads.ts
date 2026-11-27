import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { apiFetch, apiFetchRaw } from '@/lib/api/fetch'
import { ApiError } from '@/lib/errors/api-error'
import { chatBaseOn, repoChatsBaseForWorkspace } from '@/lib/workspace-scope-url'

export function chatBase(wsId: string): string {
  return repoChatsBaseForWorkspace(wsId)
}

export interface AgentChat {
  id: string
  workspaceId: string
  title: string
  liveRunnerId: string
  terminalSessionId: string
  surface?: LandingChatPresentation
  activeProviderId: string
  working: boolean
  createdAt: string
  version: number
  phase: ChatPhase
  parentId?: string
  order: number
  model?: string
  effort?: string
  launchModel?: string
  launchEffort?: string
  permissionLevel?: PermissionLevel
  terminalWait?: AgentTerminalWait
  pendingMove?: PendingMove
}

interface PendingMove {
  requestId: string
  workspaceId?: string
  parentId?: string
  label?: string
}

type ChatPhase = 'dormant' | 'starting' | 'live' | 'switching' | 'stopping'

export interface AgentTerminalWait {
  kind: string
  answers?: AgentTerminalAnswer[]
}

export interface AgentTerminalAnswer {
  id: string
  label: string
}

export const TERMINAL_WAIT_TRUST = 'workspace_trust'

export interface AgentChatFolder {
  id: string
  workspaceId: string
  parentId?: string
  name: string
  kind?: 'read' | 'edit' | 'execute' | 'search' | 'fetch' | 'other' | string
  order: number
}

interface AgentChatFolderWire {
  id: string
  workspaceId: string
  parentId: string
  title: string
  order: number
}

export interface FolderShift {
  shifted: AgentChatFolderWire[]
}

type SlashCatalogCompleteness = 'complete' | 'model_visible' | 'plugin_only'

export interface SlashCatalogItem {
  id: string
  kind: 'skill' | 'builtin' | 'command'
  label: string
  description: string
  insertText: string
  source: string
  terminal?: boolean
}

export interface SlashCatalog {
  providerId: string
  completeness: SlashCatalogCompleteness
  items: SlashCatalogItem[]
  warnings: string[]
}

export function mapChat(c: AgentChat): AgentChat {
  return {
    id: c.id,
    workspaceId: c.workspaceId,
    title: c.title,
    liveRunnerId: c.liveRunnerId,
    terminalSessionId: c.terminalSessionId,
    surface: c.surface,
    activeProviderId: c.activeProviderId,
    working: c.working,
    createdAt: c.createdAt,
    version: c.version,
    phase: c.phase,
    parentId: c.parentId ?? '',
    order: c.order ?? 0,
    model: c.model ?? '',
    effort: c.effort ?? '',
    launchModel: c.launchModel ?? '',
    launchEffort: c.launchEffort ?? '',
    permissionLevel: c.permissionLevel,
    terminalWait: c.terminalWait
      ? { kind: c.terminalWait.kind ?? '', answers: c.terminalWait.answers }
      : undefined,
    pendingMove: c.pendingMove,
  }
}

function mapFolder(f: AgentChatFolderWire): AgentChatFolder {
  return {
    id: f.id,
    workspaceId: f.workspaceId,
    parentId: f.parentId ?? '',
    name: f.title,
    order: f.order ?? 0,
  }
}

export function mapShift(raw: Partial<FolderShift> | null): AgentChatFolder[] {
  return (raw?.shifted ?? []).map(mapFolder)
}

export async function readBlob(
  hostId: string,
  chatId: string,
  ref: string,
  signal?: AbortSignal,
): Promise<string | null> {
  const url = `${chatBaseOn(hostId, chatId)}/blobs/${encodeURIComponent(ref)}`
  try {
    return await (await apiFetchRaw(url, { signal })).text()
  } catch (err: unknown) {
    if (err instanceof ApiError && err.status === 404) return null
    throw err
  }
}

interface AgentContextUsage {
  capacityTokens?: number
  usedTokens?: number
  usedPercent?: number
  remainingPercent?: number
}

interface AgentRateLimit {
  id: string
  label?: string
  usedPercent?: number
  resetsAt?: string
}

export interface AgentTelemetry {
  observedAt: string
  source: 'callback' | 'probe'
  context?: AgentContextUsage
  rateLimits?: AgentRateLimit[]
  cost?: { totalUsd?: number; apiDurationMs?: number }
  model?: { id: string; displayName?: string }
}

export async function getSlashCatalog(
  wsId: string,
  id: string,
  signal?: AbortSignal,
): Promise<SlashCatalog> {
  const raw = await apiFetch<SlashCatalog>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/slash-catalog`,
    { signal },
    { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 },
  )
  return { ...raw, items: raw?.items ?? [], warnings: raw?.warnings ?? [] }
}
