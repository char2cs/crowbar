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
  /** The provider's CLI is installed — its spawn.cmd resolves on PATH. Install-only
   *  (no auth probe); informational for the New-chat pick, which follows priority. */
  connected: boolean
  /** The provider is offered — `!disabled` in the global preference. A disabled
   *  provider drops out of every New-chat surface. */
  enabled: boolean
  /** Crowbar registers its own tool surface with this provider — `!mcpDisabled` in
   *  the global preference. A SEPARATE axis from `enabled`: a provider with its
   *  tools switched off still spawns, still fires its hooks and still holds a
   *  normal chat; it just cannot reach into Crowbar. */
  mcpEnabled: boolean
  /**
   * Whether this provider's descriptor declares a model / effort catalogue AT ALL.
   *
   * False means the picker DOES NOT EXIST for it — absent UI, never a disabled
   * control implying breakage.
   */
  modelSelect: boolean
  effortSelect: boolean
  /** Canonical rich events declared by this provider's descriptor. */
  activityEvents?: string[]
  /**
   * The provider declares a compaction gesture (`compact_start`) — claude's
   * `/compact` injection, or an API transport's own call.
   *
   * Key presence, like everything else here: a provider that declares none gets
   * NO compact control, and `POST /compact` answers 404 for it.
   */
  compaction: boolean
  /**
   * Whether this provider's terminal surface EXISTS AT ALL — structural, not a
   * capability the descriptor opts into.
   */
  hasTerminal: boolean
  /**
   * Whether this provider's chat and terminal faces can be live at the same
   * instant. When false the user is asked to finish the turn rather than
   * being handed a swap nobody verified.
   */
  hotswap: boolean
  /**
   * A message sent while a turn runs is delivered INTO that turn by the daemon,
   * so the composer need not hold it until the turn ends.
   */
  promptSteer?: boolean
  /**
   * Whether a BRAND-NEW chat may be launched DIRECTLY onto this provider's
   * terminal surface, rather than reached only by switching to it after a
   * turn (design spec 2.5's `surfaces.terminal.start_here`). `hasTerminal`
   * says the surface exists at all; this says it may be a LANDING surface.
   * Codex reports it TRUE: its terminal is the hooks-channel PTY every spawn
   * already forks (descriptor `terminal: { channel: hooks, start_here: true }`).
   * The idle-only sequential handoff is `attach` — the api channel's way back
   * onto an EXISTING session, which says nothing about landing.
   */
  terminalStartHere: boolean
  /** The declared model catalogue, in DESCRIPTOR ORDER. Never re-sorted: the
   *  order is the provider's own ranking. */
  models?: string[]
  /**
   * Effort levels keyed by model id, ALREADY RESOLVED by the backend: every entry
   * in `models` has a key, plus '' for the provider's own default model.
   *
   * Read it as `efforts[model]` where `model` is '' when the chat has no
   * selection — that is the whole rule. The descriptor's model-independent
   * fallback (claude's `"*"`) is applied server-side, so there is no fallback to
   * implement here and no provider knowledge to hardcode. A model with no levels
   * is ABSENT from the map, and absent means: draw no effort picker.
   */
  efforts?: Record<string, string[]>
  /**
   * Which of Crowbar's own guarded/trusted/full-auto levels this provider's
   * descriptor actually declares, least to most permissive. A level absent
   * here is not offered for this provider AT ALL — the server rejects an
   * explicit pick of it — never a value to grey out or silently substitute.
   * Absent/empty means this provider's picker offers nothing.
   */
  permissionLevels?: PermissionLevel[]
}

/** One row of the global provider preference set: an id and both of its NEGATIVE
 *  flags. Negative because that is what the row stores rather than what the switch
 *  shows, so an omitted field means the default in the same direction the DB does.
 *  The submission order (index) defines the priority — see updateProviderPreferences.
 *
 *  BOTH flags travel on EVERY row of EVERY write. The backend replaces whole rows,
 *  so a payload that names only `disabled` does not leave `mcpDisabled` alone — it
 *  writes the zero value over it, and a reorder would silently switch a provider's
 *  tool surface back on. */
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

/**
 * Whether `provider` may be the target of a chat CREATED directly on its
 * terminal surface — the shared gate behind every "New thread in Terminal"
 * affordance (the sidebar row menu, the space header's overflow menu, the
 * ⌥⌘N chord). `hasTerminal` says the surface exists at all; `terminalStartHere`
 * says it may be LAUNCHED INTO rather than reached only by switching to it
 * after a turn (design spec 2.5's `start_here`; both shipped descriptors
 * declare it today, so this gate only ever refuses a provider that does not).
 * Both must hold: house rule is absence, not a disabled control, so a provider
 * silent on either gets no affordance at all.
 *
 * `provider` undefined (no enabled provider resolved yet) reads permissive —
 * the plain Thread button offers itself unconditionally too and leaves the
 * refusal to click-time resolution; this matches it.
 */
export function providerCanStartOnTerminal(provider: AgentProvider | undefined): boolean {
  if (!provider) return true
  return provider.hasTerminal && provider.terminalStartHere
}

/**
 * The `surface` a CREATE names when its caller named none: the user's own
 * default landing presentation, gated by the same rule above.
 *
 * That default used to be consulted only at DISPLAY time
 * (use-chat-presentation.ts), so with "native chats" off a codex chat was
 * forked on its api face — no PTY — and the pane then asked for a terminal
 * view that had never been created ("This agent has no terminal view attached
 * right now").
 *
 * An UNRESOLVED provider reads as no surface, unlike `providerCanStartOnTerminal`
 * itself: permissive is right for offering an affordance a click can retry, and
 * wrong for a create, which forks the face it names and cannot take it back.
 */
export function createSurfaceFor(
  provider: AgentProvider | undefined,
  explicit?: LandingChatPresentation,
): LandingChatPresentation | undefined {
  if (explicit) return explicit
  if (!provider || !providerCanStartOnTerminal(provider)) return undefined
  return getDefaultChatPresentation() === 'terminal' ? 'terminal' : undefined
}

// updateProviderPreferences rewrites the GLOBAL provider preference set. The body
// is the COMPLETE ordered list of known providers — array index becomes priority,
// `disabled` toggles enablement — PUT to the settings route (not workspace-scoped:
// provider priority/enabled is machine-level). The response is the freshly resolved
// list (same enrichment + order as listProviders), so the caller reconciles from
// server truth with no second fetch.
export async function updateProviderPreferences(
  prefs: ProviderPreference[],
): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(`/v0/settings/chat/providers`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ providers: prefs }),
  })
  return (raw ?? []).map(mapProvider)
}

export async function listGlobalProviders(): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>('/v0/settings/chat/providers')
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
}> = [
  { value: 'guarded', label: 'Guarded' },
  { value: 'trusted', label: 'Trusted' },
  { value: 'full-auto', label: 'Full Auto' },
]

/** One problem the daemon's descriptor validator found. */
interface DescriptorFinding {
  rule: string
  severity: 'error' | 'warning'
  /** YAML path, e.g. `session.locate.glob[0]`. */
  path: string
  line: number
  message: string
  hint?: string
}

/** A descriptor's static validation; any error means the daemon will not enable it. */
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

export async function getDefaultPermissionLevel(): Promise<PermissionLevel> {
  const res = await apiFetch<{ level: PermissionLevel }>(`/v0/settings/chat/permission-level`)
  return res.level
}

export async function updateDefaultPermissionLevel(
  level: PermissionLevel,
): Promise<PermissionLevel> {
  const res = await apiFetch<{ level: PermissionLevel }>(`/v0/settings/chat/permission-level`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ level }),
  })
  return res.level
}
