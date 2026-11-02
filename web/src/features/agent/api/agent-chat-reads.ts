import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { apiFetch, apiFetchRaw } from '@/lib/api/fetch'
import { ApiError } from '@/lib/errors/api-error'
import { repoChatsBaseForWorkspace } from '@/lib/workspace-scope-url'

// Agentic-chat REST client. A chat is no longer addressed through its
// workspace (Task 17 rescope, model spec §5.1): a non-home workspace's chats
// nest under its REPO — .../projects/:p/repos/:r/chats — while a home
// workspace still routes through the still-live workspaceBase(wsId)/home
// mount, since home has no repo. The {success,data} envelope is unwrapped by
// apiFetch. Modelled on features/git/api/review-api.ts.

// Exported so callers that build their OWN chat-scoped URL — the agent-chat
// lifecycle WS subscription (use-workspace-agent-chats-stream.ts) is the one
// today — derive the same repo-scoped/home branch this file's own routes do,
// rather than re-deriving it (and drifting from it, as that hook's direct
// `workspaceBase(wsId)/chats` build had, post Task 17).
//
// The shape itself now lives beside the other scope-to-URL builders, because
// the worktree LIFECYCLE verbs mount on this same repo-scoped chat prefix and
// must not re-derive it here.
export function chatBase(wsId: string): string {
  return repoChatsBaseForWorkspace(wsId)
}

// ── Wire shapes (identical to the backend DTOs; camelCase) ──────────

// AgentChat carries three DERIVED runner facts the backend joins on at read time;
// none of them is stored on the chat.
//
// liveRunnerId is the WHOLE liveness contract. It names the runner (the vendor-CLI
// process) currently placed on this chat, and it is '' exactly when the chat is
// dormant — nobody is talking to it, because its CLI exited or died with the
// daemon. There is deliberately no status/isLive flag anywhere on this shape: a
// second authority on liveness could only drift from the process, and that drift
// is the production bug this model exists to delete. A live-runner row exists
// exactly while its PTY does, so the PRESENCE of liveRunnerId IS the answer — a
// client needs no second call to confirm it.
export interface AgentChat {
  id: string
  workspaceId: string
  title: string
  /** The runner placed on this chat, or '' when the chat is dormant. */
  liveRunnerId: string
  /** That runner's PTY — what a chat pane attaches to. '' exactly when liveRunnerId is. */
  terminalSessionId: string
  /**
   * The VIEW this chat is on RIGHT NOW (design spec 2.5). Creating it seeds
   * this, and `switchToTerminal`/`switchToNative` move it — it never says how
   * the chat got here. OMITTED for the overwhelming majority, which sit on
   * their provider's own default face.
   *
   * 'terminal' means the daemon has NO api connection for this chat, so
   * `terminalSessionId` above IS its conversation.
   */
  surface?: LandingChatPresentation
  /** The live runner's provider, else the provider of the chat's LAST conversation
   *  (so a dormant chat still shows the right glyph, and Resume knows who to bring
   *  back). '' only on a chat no runner has ever been placed on. */
  activeProviderId: string
  /** Server-folded turn state — the chat aggregate's own fold, carried whole on
   *  every snapshot. Never derived client-side. */
  working: boolean
  createdAt: string
  /**
   * Orders every answer the daemon gives about this chat, frame or GET: a
   * snapshot applies only if its version is newer than the one held.
   * Larger across daemon restarts too.
   */
  version: number
  /** The chat's lifecycle phase, owned by the daemon. The pane renders it. */
  phase: ChatPhase
  /**
   * The row this chat hangs off — another CHAT (making this one a thread of it) or
   * a FOLDER. '' is the workspace root.
   *
   * Domain truth, not a view preference: a thread reads its parent's turns, so
   * parentage decides what an agent SEES. It is stored on the aggregate and it is
   * why the panel's arrangement is no longer in localStorage.
   */
  parentId?: string
  /** Dense index within this chat's sibling space, SHARED with folders. */
  order: number
  /**
   * The chat's STICKY choice of what to run its provider CLI as — the model id
   * and the reasoning effort, exactly as the provider's descriptor spells them.
   *
   * Both are OMITTED on the wire when unset, and unset is a real answer:
   * "whatever this provider defaults to". It is not a value anything may render
   * as selected, because Crowbar does not know what that default resolves to —
   * only the CLI does. They are grounded to '' here so an absent key and a
   * cleared one are the same thing everywhere downstream.
   *
   * This is the REQUEST. What a turn actually ran under is a separate fact the
   * provider reports on the turn itself (its turn_ended row's effort).
   */
  model?: string
  effort?: string
  /**
   * What the LIVE runner actually spawned as — Crowbar's own record of the
   * resolved argv, which can differ from model/effort above (a provider or an
   * organization's own config can rewrite a request). Absent exactly when
   * liveRunnerId is: a dormant chat has no live spawn to report.
   */
  launchModel?: string
  launchEffort?: string
  /**
   * Set exactly while this chat's CLI is blocked on a prompt Crowbar CANNOT
   * answer — a workspace-trust dialog, a first-run setup screen, a login — that
   * reaches the daemon through no hook. Absent otherwise, which is the common
   * case and the permanent case for a provider that declares no such prompts.
   *
   * It is the COMPLEMENT of AgentChoice. A choice is a question with buttons the
   * chat can press; this is a signpost, and the only correct response to it is to
   * go to the terminal. Detected server-side against the daemon's own screen
   * model — never scraped in the browser.
   */
  terminalWait?: AgentTerminalWait
}

/** Where a chat is in its lifecycle. `dormant` and `live` follow placement;
 *  the other three are an operation the daemon is running on it right now. */
type ChatPhase = 'dormant' | 'starting' | 'live' | 'switching' | 'stopping'

/** What a chat's CLI is blocked on that Crowbar has no channel to answer.
 *
 *  `kind` is Crowbar's own name for the prompt, and it is '' whenever the daemon
 *  recognised only that SOMETHING is up. A client with no kind says exactly that
 *  and does not guess — and a client seeing a kind it does not know does the
 *  same, so a daemon that learns a new kind never makes an older client say
 *  something wrong. */
export interface AgentTerminalWait {
  kind: string
}

/** Crowbar's names for the prompts it recognises specifically. Anything else —
 *  including a kind minted by a newer daemon — falls back to the generic wording. */
export const TERMINAL_WAIT_TRUST = 'workspace_trust'

/**
 * A grouping row in the Chats tree.
 *
 * It holds no turns, which is the whole reason it may live anywhere a chat may —
 * including inside a chat, where it orders that chat's threads. Context lineage
 * is the ancestor chain filtered to chats and steps straight through one, so
 * filing threads into folders cannot change what any of them reads.
 */
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

/**
 * The wire shape of one .../chats/folders row: a folder-typed domain.Chat
 * rendered through dto.AgentChatDTO, which names its display text `title` —
 * not `name`. Only the fields a folder row carries values in; every
 * runner-derived AgentChatDTO field is honestly empty on one and unused here.
 */
interface AgentChatFolderWire {
  id: string
  workspaceId: string
  parentId: string
  title: string
  order: number
}

/**
 * The rows a dense renumber MOVED that the caller did not ask about.
 *
 * Every placement write renumbers the level it touched, so a client that applies
 * only its own row holds stale orders for every sibling until the next reconnect —
 * and stale orders are a list that re-sorts itself into the wrong sequence the
 * next time anything re-renders. Apply these.
 */
export interface FolderShift {
  shifted: AgentChatFolderWire[]
}

type SlashCatalogCompleteness = 'complete' | 'model_visible' | 'plugin_only'

export interface SlashCatalogItem {
  id: string
  kind: 'skill'
  label: string
  description: string
  insertText: string
  source: string
}

export interface SlashCatalog {
  providerId: string
  completeness: SlashCatalogCompleteness
  items: SlashCatalogItem[]
  warnings: string[]
}

// ── Mappers (wire → store types). Identity today, but kept explicit so a
//    future wire/store divergence changes one place (review-api idiom). ──
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
    // Grounded here, once, so nothing downstream has to remember that an absent
    // parent and a root parent are the same thing. `order` defaults to 0, which
    // ties every chat on a daemon that has not placed them yet — the tree breaks
    // that tie by recency, which is the arrangement the panel had before.
    parentId: c.parentId ?? '',
    order: c.order ?? 0,
    // Same grounding as parentId, for the same reason: '' is the value the
    // selection endpoint takes to mean "back to the provider's own default", so
    // an omitted field and a cleared one must not be two different things.
    model: c.model ?? '',
    effort: c.effort ?? '',
    // Same grounding, and the same reason mapChat exists at all: an explicit
    // field list is how launchModel/launchEffort ALREADY being on the wire and
    // on the AgentChat type still never reached the store — this function
    // narrows to exactly what it lists, silently, and these two were missing
    // from it. Confirmed live: the daemon's own response carried a real
    // launchModel while every chat in the running app kept showing "unset".
    launchModel: c.launchModel ?? '',
    launchEffort: c.launchEffort ?? '',
    // NOT grounded to a value: absent IS the answer here (nothing is blocking
    // this chat), so it stays undefined rather than becoming a falsy object that
    // every reader would then have to interrogate. `kind` inside it IS grounded,
    // because an identified prompt and an unidentified one are both real answers.
    terminalWait: c.terminalWait ? { kind: c.terminalWait.kind ?? '' } : undefined,
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
  chatId: string,
  ref: string,
  signal?: AbortSignal,
): Promise<string | null> {
  const url = `/v0/chats/${encodeURIComponent(chatId)}/blobs/${encodeURIComponent(ref)}`
  try {
    return await (await apiFetchRaw(url, { signal })).text()
  } catch (err: unknown) {
    if (err instanceof ApiError && err.status === 404) return null
    throw err
  }
}

/** What the provider itself reported about cost and capacity.
 *
 *  Every field is OPTIONAL because "not reported" and "zero" are different facts:
 *  a gauge rendering 0% for something the provider never sent is a lie, so an
 *  absent field must render as no gauge rather than an empty one. */
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
