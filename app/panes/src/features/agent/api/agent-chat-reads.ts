import type { PermissionLevel } from '@/features/agent/api/agent-providers-api'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { apiFetch, apiFetchRaw } from '@/lib/api/fetch'
import { ApiError } from '@/lib/errors/api-error'
import { chatBaseOn, repoChatsBaseForWorkspace } from '@/lib/workspace-scope-url'

// Agentic-chat REST client (model spec §5.1). A repo workspace's chats nest under its repo
// (.../projects/:p/repos/:r/chats); a home workspace, which has no repo, uses
// workspaceBase(wsId)/chats. apiFetch unwraps the {success,data} envelope.

// The chat URL prefix for `wsId`, for callers building their own chat-scoped URL (e.g. the
// lifecycle WS stream) so they can't drift from these routes. The shape lives in
// workspace-scope-url.ts beside the worktree lifecycle verbs that share it.
export function chatBase(wsId: string): string {
  return repoChatsBaseForWorkspace(wsId)
}

// ── Wire shapes (identical to the backend DTOs; camelCase) ──────────

// Runner facts are DERIVED at read time, never stored. liveRunnerId is the whole liveness
// contract: '' exactly when the chat is dormant (its CLI exited or died with the daemon). No
// status flag exists on purpose: a second liveness authority could only drift from the process.
export interface AgentChat {
  id: string
  workspaceId: string
  title: string
  /** The runner placed on this chat, or '' when the chat is dormant. */
  liveRunnerId: string
  /** That runner's PTY — what a chat pane attaches to. '' exactly when liveRunnerId is. */
  terminalSessionId: string
  /** The view the chat is on now (design spec 2.5): seeded at create, moved by switchToTerminal /
   *  switchToNative, omitted on the provider's default face. 'terminal': the PTY IS the chat. */
  surface?: LandingChatPresentation
  /** The live runner's provider, else the provider of the chat's LAST conversation
   *  (so a dormant chat still shows the right glyph, and Resume knows who to bring
   *  back). '' only on a chat no runner has ever been placed on. */
  activeProviderId: string
  /** Server-folded turn state — the chat aggregate's own fold, carried whole on
   *  every snapshot. Never derived client-side. */
  working: boolean
  createdAt: string
  /** Orders every daemon answer about this chat (frame or GET): a snapshot applies only if newer
   *  than the one held. Monotonic across daemon restarts. */
  version: number
  /** The chat's lifecycle phase, owned by the daemon. The pane renders it. */
  phase: ChatPhase
  /** The parent row: a CHAT (making this a thread of it) or a FOLDER; '' is the workspace root.
   *  Stored on the aggregate because it is domain truth: a thread reads its parent's turns. */
  parentId?: string
  /** Dense index within this chat's sibling space, SHARED with folders. */
  order: number
  // The chat's sticky REQUEST for model and effort, spelled as the provider's descriptor does.
  // Unset (omitted, grounded to '') means "the provider's default", which only the CLI can resolve,
  // so it is never rendered as a selection. What a turn ran under is on its turn_ended row.
  model?: string
  effort?: string
  /** What the live runner actually spawned as (resolved argv); provider or org config can rewrite
   *  the request. Absent exactly when liveRunnerId is. */
  launchModel?: string
  launchEffort?: string
  /** The level this chat is pinned to; absent while it inherits the default. */
  permissionLevel?: PermissionLevel
  /** Set while the CLI is blocked on a prompt with no hook (workspace trust, first-run, login).
   *  Unlike AgentChoice, it has no buttons the chat can press: the answer is to go to the terminal.
   *  Detected server-side against the daemon's screen model. */
  terminalWait?: AgentTerminalWait
  /** A move to another workspace waiting for the chat's turn boundary; gone once it ran. */
  pendingMove?: PendingMove
}

interface PendingMove {
  requestId: string
  /** The workspace the chat runs in once the move runs. */
  workspaceId?: string
  parentId?: string
  /** Where, in the sidebar's words. */
  label?: string
}

/** Where a chat is in its lifecycle. `dormant` and `live` follow placement;
 *  the other three are an operation the daemon is running on it right now. */
type ChatPhase = 'dormant' | 'starting' | 'live' | 'switching' | 'stopping'

/** What a chat's CLI is blocked on that Crowbar has no channel to answer. `kind` '' means only
 *  "something is up"; an unknown kind is treated the same, so a newer daemon's kinds never make
 *  an older client say something wrong. */
export interface AgentTerminalWait {
  kind: string
  /** One-tap replies the provider's descriptor declares for this prompt. Absent
   *  or empty means the terminal is the only way past it. */
  answers?: AgentTerminalAnswer[]
}

export interface AgentTerminalAnswer {
  id: string
  /** The button text, from the descriptor. */
  label: string
}

/** Crowbar's names for the prompts it recognises specifically. Anything else —
 *  including a kind minted by a newer daemon — falls back to the generic wording. */
export const TERMINAL_WAIT_TRUST = 'workspace_trust'

/** A grouping row in the Chats tree. It holds no turns, so it may live anywhere a chat may (even
 *  inside one, ordering its threads); context lineage steps through it, so filing threads into
 *  folders never changes what they read. */
export interface AgentChatFolder {
  id: string
  workspaceId: string
  /** A chat id, a folder id, or '' at the workspace root. */
  parentId?: string
  name: string
  /** Provider-neutral semantic category declared by the descriptor. */
  kind?: 'read' | 'edit' | 'execute' | 'search' | 'fetch' | 'other' | string
  /** Dense index within its sibling space, SHARED with chats. */
  order: number
}

/** One .../chats/folders row: a folder-typed chat rendered through AgentChatDTO, so its name
 *  arrives as `title`. Only the fields a folder carries; the runner fields are empty on it. */
interface AgentChatFolderWire {
  id: string
  workspaceId: string
  parentId: string
  title: string
  order: number
}

/** Siblings a placement write's dense renumber moved. Apply them, or the caller holds stale
 *  orders and the list re-sorts into the wrong sequence on the next render. */
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
  /** Running it opens a dialog only the CLI's terminal shows. */
  terminal?: boolean
}

export interface SlashCatalog {
  providerId: string
  completeness: SlashCatalogCompleteness
  items: SlashCatalogItem[]
  warnings: string[]
}

// ── Mappers (wire → store): an explicit field list; anything unlisted never reaches the store.
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
    // Grounded once so an absent parent and a root parent are the same thing. `order` 0 ties
    // unplaced chats; the tree breaks the tie by recency.
    parentId: c.parentId ?? '',
    order: c.order ?? 0,
    // '' is what the selection endpoint takes for "the provider's default", so an omitted field
    // and a cleared one must be the same.
    model: c.model ?? '',
    effort: c.effort ?? '',
    // Listed explicitly: the field list narrows silently, so a missing entry drops the field.
    launchModel: c.launchModel ?? '',
    launchEffort: c.launchEffort ?? '',
    permissionLevel: c.permissionLevel,
    // Absent IS the answer (nothing blocking), so it stays undefined; `kind` is grounded because
    // identified and unidentified prompts are both real answers.
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

/** The `shifted` half of every folder write, grounded the same way. */
export function mapShift(raw: Partial<FolderShift> | null): AgentChatFolder[] {
  return (raw?.shifted ?? []).map(mapFolder)
}

/** One content-addressed payload a row names (`sha256:…`); null once swept. */
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

/** What the provider reported about cost and capacity. All optional: "not reported" is not zero,
 *  so an absent field renders no gauge rather than an empty one. */
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

/** Run the active provider's deterministic, descriptor-declared skill probe.
 *  Reads are deliberately single-attempt when cancellable: apiFetch's ordinary
 *  cold-start retry would otherwise replay an already-aborted request. */
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
