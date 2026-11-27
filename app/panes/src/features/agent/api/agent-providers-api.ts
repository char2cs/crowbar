import {
  getDefaultChatPresentation,
  type LandingChatPresentation,
} from '@/features/settings/lib/chat-presentation'
import { apiFetch } from '@/lib/api/fetch'
import { chatBase } from '@/features/agent/api/agent-chat-reads'

export interface AgentProvider {
  id: string
  displayName: string
  icon: string
  connected: boolean
  enabled: boolean
  mcpEnabled: boolean
  modelSelect: boolean
  effortSelect: boolean
  activityEvents?: string[]
  compaction: boolean
  shellStop?: boolean
  hasTerminal: boolean
  hotswap: boolean
  promptSteer?: boolean
  terminalStartHere: boolean
  signIn?: string
  models?: string[]
  defaultModel?: string
  efforts?: Record<string, string[]>
  defaultEfforts?: Record<string, string>
  permissionLevels?: PermissionLevel[]
}

export interface ProviderPreference {
  id: string
  disabled: boolean
  mcpDisabled: boolean
}

function mapProvider(p: AgentProvider): AgentProvider {
  return {
    ...p,
    models: p.models ?? [],
    efforts: p.efforts ?? {},
    permissionLevels: p.permissionLevels ?? [],
  }
}

export async function listProviders(wsId: string): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(`${chatBase(wsId)}/providers`)
  return (raw ?? []).map(mapProvider)
}

export function providerCanStartOnTerminal(provider: AgentProvider | undefined): boolean {
  if (!provider) return true
  return provider.hasTerminal && provider.terminalStartHere
}

export function createSurfaceFor(
  provider: AgentProvider | undefined,
  explicit?: LandingChatPresentation,
): LandingChatPresentation | undefined {
  if (explicit) return explicit
  if (!provider || !providerCanStartOnTerminal(provider)) return undefined
  return getDefaultChatPresentation() === 'terminal' ? 'terminal' : undefined
}

export async function updateProviderPreferences(
  prefs: ProviderPreference[],
  hostId?: string,
): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(
    `/v0/settings/chat/providers`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ providers: prefs }),
    },
    undefined,
    hostId,
  )
  return (raw ?? []).map(mapProvider)
}

export async function listGlobalProviders(hostId?: string): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(
    '/v0/settings/chat/providers',
    undefined,
    undefined,
    hostId,
  )
  return (raw ?? []).map(mapProvider)
}

export type PermissionLevel = 'guarded' | 'trusted' | 'full-auto'

export const PERMISSION_LEVEL_OPTIONS: ReadonlyArray<{
  value: PermissionLevel
  label: string
  description: string
}> = [
  { value: 'guarded', label: 'Guarded', description: 'Asks before edits, commands and tools.' },
  { value: 'trusted', label: 'Trusted', description: 'Edits files freely. Asks before commands.' },
  {
    value: 'full-auto',
    label: 'Full Auto',
    description: 'Never asks. Runs every command and tool.',
  },
]

interface DescriptorFinding {
  rule: string
  severity: 'error' | 'warning'
  path: string
  line: number
  message: string
  hint?: string
}

export interface DescriptorReport {
  id: string
  source?: string
  findings: DescriptorFinding[]
  fellBack?: boolean
}

export async function getDescriptorReports(): Promise<DescriptorReport[]> {
  const raw = await apiFetch<DescriptorReport[]>(`/v0/settings/chat/descriptors`)
  return (raw ?? []).map((r) => ({ ...r, findings: r.findings ?? [] }))
}

export async function uploadDescriptor(file: File): Promise<DescriptorReport> {
  const body = new Uint8Array(await file.arrayBuffer())
  return apiFetch<DescriptorReport>('/v0/settings/chat/descriptors', {
    method: 'POST',
    headers: { 'Content-Type': 'application/yaml' },
    body,
  })
}

export async function getDefaultPermissionLevel(hostId?: string): Promise<PermissionLevel> {
  const res = await apiFetch<{ level: PermissionLevel }>(
    `/v0/settings/chat/permission-level`,
    undefined,
    undefined,
    hostId,
  )
  return res.level
}

export async function updateDefaultPermissionLevel(
  level: PermissionLevel,
  hostId?: string,
): Promise<PermissionLevel> {
  const res = await apiFetch<{ level: PermissionLevel }>(
    `/v0/settings/chat/permission-level`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ level }),
    },
    undefined,
    hostId,
  )
  return res.level
}
