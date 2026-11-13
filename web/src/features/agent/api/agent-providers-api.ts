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
  /** The CLI resolves on PATH (no auth probe); informational, since the New-chat
   *  pick follows priority. */
  connected: boolean
  /** `!disabled` in the global preference; a disabled provider leaves every
   *  New-chat surface. */
  enabled: boolean
  /** `!mcpDisabled`: Crowbar's tools are registered. Separate from `enabled` —
   *  with tools off the provider still runs a normal chat, it just cannot reach
   *  into Crowbar. */
  mcpEnabled: boolean
  /** False means no picker at all — never a disabled control implying breakage. */
  modelSelect: boolean
  effortSelect: boolean
  activityEvents?: string[]
  /** Without a declared `compact_start` there is no compact control, and
   *  `POST /compact` answers 404. */
  compaction: boolean
  /** The descriptor declares how to ask the agent to end a background shell; without it
   *  a shell row has no stop control and `POST /shells/:id/stop` answers 404. */
  shellStop?: boolean
  /** Structural, not a capability the descriptor opts into. */
  hasTerminal: boolean
  /** Chat and terminal faces can be live at once; when false the user finishes
   *  the turn first rather than getting an unverified swap. */
  hotswap: boolean
  /** The daemon delivers a mid-turn message into that turn, so the composer
   *  need not hold it. */
  promptSteer?: boolean
  /** A new chat may LAND on the terminal, not just switch to it after a turn;
   *  `hasTerminal` only says the surface exists. */
  terminalStartHere: boolean
  /** The shell command that signs this provider's CLI in, when it declares one. */
  signIn?: string
  /** Never re-sorted: descriptor order is the provider's own ranking. */
  models?: string[]
  /** The provider's own stated default; absent when it states none, never inferred from `models`. */
  defaultModel?: string
  /** A chat with no model pick cannot start: the provider reports its own selection and has none. */
  modelRequired?: boolean
  /** Resolved server-side, fallbacks included: read `efforts[model]` with '' for
   *  no selection. A model with no levels is absent, meaning no effort picker. */
  efforts?: Record<string, string[]>
  /** Each model's stated default effort, which a chat with effort '' runs at;
   *  a model stating none is absent (unknown, not its first level). */
  defaultEfforts?: Record<string, string>
  /** Declared levels, least to most permissive. An absent level is not offered
   *  at all (the server rejects it) — never greyed out or substituted. */
  permissionLevels?: PermissionLevel[]
}

/** Negative flags mirror what the DB stores, so an omitted field defaults the same
 *  way. Both travel on every row: the backend replaces whole rows, and a missing
 *  `mcpDisabled` would silently re-enable a provider's tools. */
export interface ProviderPreference {
  id: string
  disabled: boolean
  mcpDisabled: boolean
}

// Every capability flag is always on the wire (the DTO has no omitempty on
// them); only the catalogues are omitted when empty, and no-catalogue must
// render as no picker rather than an empty one.
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

/** Gate for every "New thread in Terminal" affordance. An unresolved provider
 *  reads permissive, matching the plain Thread button, which leaves refusal to
 *  click-time resolution. */
export function providerCanStartOnTerminal(provider: AgentProvider | undefined): boolean {
  if (!provider) return true
  return provider.hasTerminal && provider.terminalStartHere
}

/** Applies the default landing presentation at create time, so a terminal-default
 *  chat is forked with a PTY. An unresolved provider gets no surface: a create
 *  forks the face it names and cannot take it back. */
export function createSurfaceFor(
  provider: AgentProvider | undefined,
  explicit?: LandingChatPresentation,
): LandingChatPresentation | undefined {
  if (explicit) return explicit
  if (!provider || !providerCanStartOnTerminal(provider)) return undefined
  return getDefaultChatPresentation() === 'terminal' ? 'terminal' : undefined
}

// The body is the complete ordered list (index = priority); preferences are
// machine-level, and the response is the resolved list so no second fetch is needed.
// Given `hostId`, that remote host's daemon's set instead of the local one's.
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

/** The providers the local daemon — or, given `hostId`, that remote host's daemon — offers.
 *  Providers belong to a daemon, so this route needs no workspace to be addressed. */
export async function listGlobalProviders(hostId?: string): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(
    '/v0/settings/chat/providers',
    undefined,
    undefined,
    hostId,
  )
  return (raw ?? []).map(mapProvider)
}

/** How much of a new chat's tool-call approval is answered automatically —
 *  the daemon's global default, applied the moment a chat starts. */
export type PermissionLevel = 'guarded' | 'trusted' | 'full-auto'

/** The three levels' {value, label} pairs, shared by the Settings default-level
 *  row and the per-chat switcher so the two pickers can never drift apart. */
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
  /** YAML path, e.g. `session.locate.glob[0]`. */
  path: string
  line: number
  message: string
  hint?: string
}

/** Any error finding means the daemon will not enable the descriptor. */
export interface DescriptorReport {
  id: string
  /** The override file, absent for the shipped descriptor. */
  source?: string
  findings: DescriptorFinding[]
  /** The override at `source` was refused for its errors; the shipped descriptor runs. */
  fellBack?: boolean
}

export async function getDescriptorReports(): Promise<DescriptorReport[]> {
  const raw = await apiFetch<DescriptorReport[]>(`/v0/settings/chat/descriptors`)
  return (raw ?? []).map((r) => ({ ...r, findings: r.findings ?? [] }))
}

export async function uploadDescriptor(file: File): Promise<DescriptorReport> {
  // WKWebView hands Blob-backed bodies to Crowbar's custom-scheme proxy as an
  // unread stream. Materialize the file first, as chat attachments do.
  const body = new Uint8Array(await file.arrayBuffer())
  return apiFetch<DescriptorReport>('/v0/settings/chat/descriptors', {
    method: 'POST',
    headers: { 'Content-Type': 'application/yaml' },
    body,
  })
}

/** The default permission level of the local daemon, or of `hostId`'s. */
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
