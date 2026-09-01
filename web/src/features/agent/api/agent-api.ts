import { API_BASE, apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { getWorkspaceScope } from '@/lib/workspace-scope'
import { clearPersistedPromptQueue } from '@/features/agent/lib/prompt-queue-persistence'

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
export function chatBase(wsId: string): string {
  const scope = getWorkspaceScope(wsId)
  // Null scope and a home workspace (repoId '') both fall back to
  // workspaceBase, which already resolves the /home shape and already throws
  // on an unrecorded scope — no need to duplicate either behavior here.
  if (!scope || !scope.repoId) return `${workspaceBase(wsId)}/chats`
  const p = encodeURIComponent(scope.projectId)
  const r = encodeURIComponent(scope.repoId)
  return `/v0/projects/${p}/repos/${r}/chats`
}

// ── Wire shapes (identical to the backend DTOs; camelCase) ──────────

/** One conversation a chat has hosted — append-only history, oldest first. It is
 *  what a "segment" really was, minus everything that described a process (no
 *  status, no PTY, no runner id), so it cannot drift from reality. */
export interface ChatConversation {
  chatId: string
  providerId: string
  sessionId: string
  firstSeenAt: string
}

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
  /** The live runner's provider, else the provider of the chat's LAST conversation
   *  (so a dormant chat still shows the right glyph, and Resume knows who to bring
   *  back). '' only on a chat no runner has ever been placed on. */
  activeProviderId: string
  /** Server-folded turn state. The workspace stream mirrors this into its
   *  dedicated working map; carrying it on reads makes reconnect/recheck
   *  authoritative too. */
  working?: boolean
  createdAt: string
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
   * provider reports on the message itself (AgentChatMessage.effort).
   */
  model?: string
  effort?: string
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

export interface AgentChatDetail extends AgentChat {
  conversations: ChatConversation[]
}

/**
 * Who put a message's text into the chat.
 *
 * Four, not two, because four distinct authors can. `harness` is text the vendor
 * CLI injected into its own conversation as if it were a prompt (a background
 * subagent reporting back is the measured case) — the agent received it and its
 * next reply refers to it, so it is neither droppable nor the user's. `notice` is
 * Crowbar relaying a provider's own words about why a chat stopped.
 *
 * The wire carries a bare string, so a role this client does not know must still
 * render as SOMETHING rather than vanish: consumers switch on the known members
 * and fall back for the rest, never assuming the union is exhaustive.
 */
export type AgentChatMessageRole = 'user' | 'assistant' | 'harness' | 'notice'

/** One authoritative, hook-confirmed message in a Crowbar chat. */
export interface AgentChatMessage {
  /** The turn this message IS. Tool calls, subagents and interruptions attach to
   *  it, which is how a reply can show the work that produced it. */
  turnId: string
  sequence: number
  role: AgentChatMessageRole
  providerId: string
  text: string
  /**
   * The reasoning effort the PROVIDER reported for this turn — what its CLI says
   * it actually used, never what the chat asked for. Absent when the provider
   * reported none, and never inferred from the chat's selection: the two can
   * legitimately differ, and showing the request in the report's place would be
   * a fabrication.
   */
  effort?: string
  at: string
}

/** A bounded ledger page. `cursor` is the newest returned sequence and
 *  `oldestCursor` is the anchor used to page upward. */
export interface AgentChatMessagesPage {
  cursor: number
  oldestCursor: number
  hasMore: boolean
  items: AgentChatMessage[]
}

export interface AgentPromptResult {
  runnerId: string
  terminalSessionId: string
}

export type SlashCatalogCompleteness = 'complete' | 'model_visible' | 'plugin_only'

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

export interface AgentProvider {
  id: string
  displayName: string
  icon: string
  /** The provider's CLI is installed — its spawn.cmd resolves on PATH. Install-only
   *  (no auth probe); informational for the New-chat pick, which follows priority. */
  connected: boolean
  /** The provider is offered — `!disabled` in the global preference. A disabled
   *  provider drops out of every New-chat surface. Defaults to `true`. */
  enabled: boolean
  /** Crowbar registers its own tool surface with this provider — `!mcpDisabled` in
   *  the global preference. A SEPARATE axis from `enabled`: a provider with its
   *  tools switched off still spawns, still fires its hooks and still holds a
   *  normal chat; it just cannot reach into Crowbar. Defaults to `true`. */
  mcpEnabled: boolean
  /**
   * Whether this provider's descriptor declares a model / effort catalogue AT ALL.
   *
   * False means the picker DOES NOT EXIST for it — absent UI, never a disabled
   * control implying breakage. They default to false rather than true (unlike
   * mcpEnabled) because that is the safe direction here: a daemon that does not
   * send them is one whose descriptors declare no catalogue, and rendering an
   * empty picker over that would invent a capability.
   */
  modelSelect?: boolean
  effortSelect?: boolean
  /**
   * The provider declares a compaction gesture (`compact_start`) — claude's
   * `/compact` injection, or an API transport's own call.
   *
   * Key presence, like everything else here: a provider that declares none gets
   * NO compact control, and `POST /compact` answers 404 for it.
   */
  compaction?: boolean
  /**
   * Whether this provider's terminal surface EXISTS AT ALL — structural, not a
   * capability the descriptor opts into. Defaults to `true` on omission: every
   * shipped provider today spawns a real PTY, so an OLDER daemon that predates
   * this field is describing exactly that reality, and defaulting `false` would
   * hide the view switcher for every existing install until the daemon catches
   * up. This is the opposite direction from every OTHER capability key on this
   * type, deliberately: those gate a control that does not exist yet, and
   * defaulting them on hides nothing that was already there.
   */
  hasTerminal?: boolean
  /**
   * Whether this provider's chat and terminal faces can be live at the same
   * instant. Defaults to `false` on omission, the same direction as
   * modelSelect/effortSelect/compaction: an older daemon or an undeclared
   * descriptor gets the conservative answer, and the user is asked to finish
   * the turn rather than being handed a swap nobody verified.
   */
  hotswap?: boolean
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

// ── Mappers (wire → store types). Identity today, but kept explicit so a
//    future wire/store divergence changes one place (review-api idiom). ──
function mapChat(c: AgentChat): AgentChat {
  return {
    id: c.id,
    workspaceId: c.workspaceId,
    title: c.title,
    liveRunnerId: c.liveRunnerId,
    terminalSessionId: c.terminalSessionId,
    activeProviderId: c.activeProviderId,
    working: c.working ?? false,
    createdAt: c.createdAt,
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
function mapShift(raw: Partial<FolderShift> | null): AgentChatFolder[] {
  return (raw?.shifted ?? []).map(mapFolder)
}

// ── Reads ───────────────────────────────────────────────────────────
export async function listChats(wsId: string): Promise<AgentChat[]> {
  const raw = await apiFetch<AgentChat[]>(`${chatBase(wsId)}`)
  return (raw ?? []).map(mapChat)
}

export async function getChat(wsId: string, id: string): Promise<AgentChatDetail> {
  const raw = await apiFetch<AgentChatDetail>(`${chatBase(wsId)}/${encodeURIComponent(id)}`)
  return { ...mapChat(raw), conversations: raw.conversations ?? [] }
}

export interface ListChatMessagesOptions {
  after?: number
  before?: number
  limit?: number
  signal?: AbortSignal
}

/** Read hook-confirmed messages. Omitting both cursors asks for the newest page;
 *  `after` incrementally catches up and `before` pages older messages. */
export async function listChatMessages(
  wsId: string,
  id: string,
  options: ListChatMessagesOptions = {},
): Promise<AgentChatMessagesPage> {
  const query = new URLSearchParams()
  if (options.after !== undefined) query.set('after', String(options.after))
  if (options.before !== undefined) query.set('before', String(options.before))
  query.set('limit', String(options.limit ?? 100))
  const raw = await apiFetch<AgentChatMessagesPage>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/messages?${query}`,
    { signal: options.signal },
  )
  const items = raw?.items ?? []
  return {
    cursor: raw?.cursor ?? items.at(-1)?.sequence ?? 0,
    oldestCursor: raw?.oldestCursor ?? items[0]?.sequence ?? 0,
    hasMore: raw?.hasMore ?? false,
    items,
  }
}

export type ToolCallStatus = 'running' | 'ok' | 'error' | 'abandoned'

/** One tool the agent ran. The payloads are NOT here: a coding agent produces
 *  hundreds of KB per turn and almost none of it is ever opened, so each side is
 *  fetched on demand by `getToolPayload`. */
export interface AgentToolCall {
  id: string
  turnId: string
  seq: number
  name: string
  /** The file, command or URL the tool acted on, when the provider reports one.
   *  Absent is legible; a guess would be wrong. */
  target?: string
  status: ToolCallStatus
  durationMs?: number
  hasRequest: boolean
  hasResult: boolean
  startedAt: string
  endedAt?: string
}

export interface AgentSubagent {
  id: string
  turnId: string
  seq: number
  agentType?: string
  startedAt: string
  endedAt?: string
}

export type InterruptionKind =
  'permission' | 'notification' | 'elicitation' | 'compaction' | 'stopped'

/** The agent blocked on, or interrupted by, something outside the turn. These
 *  are what make an apparently frozen agent legible. */
export interface AgentInterruption {
  id: string
  turnId: string
  seq: number
  kind: InterruptionKind
  detail?: string
  at: string
  resolvedAt?: string
}

/** One answer a prompt will accept.
 *
 *  A client renders from `kind`, not from `label`: allow and deny are Crowbar's
 *  OWN words, because no provider enumerates the two answers a permission has by
 *  construction. It is a plain string rather than a union so a vocabulary the
 *  backend grows later degrades to "an option" instead of failing to compile —
 *  the four in use today are `answer`, `allow`, `deny` and `suggestion`. */
export interface AgentChoiceOption {
  id: string
  kind: string
  label?: string
  description?: string
}

/** ONE question inside a prompt, with the options that answer it.
 *
 *  `multi` is per QUESTION and not per prompt, because the provider says so:
 *  claude carries multiSelect on each entry of its questions array, so one prompt
 *  can ask "pick one" and "pick any" in the same card.
 *
 *  Option ids are unique across the WHOLE prompt, not merely within this question,
 *  because an answer names its picks in one flat list. */
export interface AgentChoiceQuestion {
  id: string
  title?: string
  text?: string
  multi?: boolean
  options: AgentChoiceOption[]
}

/**
 * The agent waiting on a HUMAN DECISION — a tool permission, a question it asked,
 * an MCP server's elicitation.
 *
 * `pending` and `answerable` are two DIFFERENT facts and both matter. `pending`
 * says the CLI is still asking. `answerable` says a relay is holding that CLI's
 * gate open right now, so an answer sent from here will actually reach it. A
 * prompt that is pending and NOT answerable is still worth drawing — the CLI is
 * genuinely asking, at its own terminal — but it must offer no buttons, because
 * they would reach nobody.
 */
export interface AgentChoice {
  id: string
  turnId: string
  seq: number
  /** `tool_permission` | `question` | `elicitation`, open for the same reason
   *  an option's kind is. */
  kind: string
  /** The tool a permission gates, when there is one. */
  toolName?: string
  title?: string
  question?: string
  /** The provider's own word for how it expects to be answered, carried verbatim
   *  and never interpreted here either. */
  mode?: string
  /** Describes `options`. A question carries its own `multi`. */
  multi?: boolean
  options: AgentChoiceOption[]
  /** The whole of what a question-kind prompt is asking, one entry per question.
   *
   *  ABSENT is meaningful and is NOT the same as empty: it says the record predates
   *  questions being modelled, and such a prompt is a single question described by
   *  `question` and `options` — render those instead of an empty card.
   *
   *  An answer covering only SOME of these is refused with 400, and that refusal is
   *  the point: a partial answer hands the CLI an input covering part of what it
   *  asked, and it goes on waiting for the rest with nothing able to send it. */
  questions?: AgentChoiceQuestion[]
  /** An elicitation's requested-input schema: the provider's own JSON, verbatim.
   *  A prompt with a schema and no options is answered with a FORM. */
  schema?: string
  pending: boolean
  answerable: boolean
  at: string
  resolvedAt?: string
  /** How it stopped pending: `answered` here, `proceeded` at the terminal, or
   *  `abandoned` with the turn. */
  resolution?: string
  /** Who answered it when `resolution` is `answered`: policy (`true`) or a
   *  human's own click (`false`). */
  autoApproved?: boolean
}

export interface AgentActivity {
  toolCalls: AgentToolCall[]
  subagents: AgentSubagent[]
  interruptions: AgentInterruption[]
  /** The prompts this chat has opened. They ride the activity payload rather than
   *  a poll of their own: a blocked agent is a state of the same turn the timeline
   *  describes, and a second loop would ask the same daemon the same question. */
  choices: AgentChoice[]
}

/** What the provider itself reported about cost and capacity.
 *
 *  Every field is OPTIONAL because "not reported" and "zero" are different facts:
 *  a gauge rendering 0% for something the provider never sent is a lie, so an
 *  absent field must render as no gauge rather than an empty one. */
export interface AgentContextUsage {
  capacityTokens?: number
  usedTokens?: number
  usedPercent?: number
  remainingPercent?: number
}

export interface AgentRateLimit {
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

export interface ListActivityOptions {
  after?: number
  limit?: number
  signal?: AbortSignal
}

/** Read what the agent DID — tool calls, subagents, interruptions — as distinct
 *  from what it said. Separate from the messages because the conversation is read
 *  on every render and this is read when a timeline is opened. */
export async function listChatActivity(
  wsId: string,
  id: string,
  options: ListActivityOptions = {},
): Promise<AgentActivity> {
  const query = new URLSearchParams()
  if (options.after !== undefined) query.set('after', String(options.after))
  if (options.limit !== undefined) query.set('limit', String(options.limit))
  const raw = await apiFetch<AgentActivity>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/activity?${query}`,
    { signal: options.signal },
  )
  return {
    toolCalls: raw?.toolCalls ?? [],
    subagents: raw?.subagents ?? [],
    interruptions: raw?.interruptions ?? [],
    choices: (raw?.choices ?? []).map(mapChoice),
  }
}

// A prompt's three decision-bearing fields are grounded here so nothing
// downstream has to treat an absent key and a false one as two different things.
// `answerable` in particular defaults FALSE: a daemon that does not send it is
// one with no answer channel at all, and defaulting it true would draw buttons
// that reach nobody — the single failure this field exists to prevent.
function mapChoice(c: AgentChoice): AgentChoice {
  return {
    ...c,
    multi: c.multi ?? false,
    options: c.options ?? [],
    pending: c.pending ?? false,
    answerable: c.answerable ?? false,
  }
}

/**
 * Answer a prompt the provider CLI is blocked on, from the chat.
 *
 * `optionIds` is ONE FLAT LIST across the whole prompt, however many questions it
 * asks: the ids say which question each pick answers, so a three-question prompt
 * is still one call. It must name at least one option (400 otherwise), the picks
 * must all be of ONE kind — "allow" and "deny" together are not an answer and no
 * template could render them — and they must COVER EVERY QUESTION. A partial
 * answer is refused with 400 rather than sent, because a CLI handed answers to two
 * of three questions goes on waiting for the third with nothing able to send it.
 *
 * A prompt that offers NO options is an elicitation, whose answer is the
 * provider's own verb — `accept`, `decline`, `cancel` — with the filled-in form in
 * `content`.
 *
 * Two failures are the caller's to SHOW rather than swallow. 400 is a decision
 * this provider cannot express (claude's `suggestion` options are the shipped
 * example: the backend declares no template for them rather than silently
 * narrowing one to a plain allow). 409 is no relay holding the gate any more — by
 * then the CLI is asking at its own terminal, and reporting success would be a lie.
 */
export async function answerChoice(
  wsId: string,
  chatId: string,
  choiceId: string,
  answer: { optionIds: string[]; reason?: string; content?: unknown },
): Promise<void> {
  await apiFetch<unknown>(
    `${chatBase(wsId)}/${encodeURIComponent(chatId)}` +
      `/choices/${encodeURIComponent(choiceId)}/answer`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      // An omitted reason/content is omitted on the wire, not sent as null: the
      // backend reads content as raw JSON and "absent" is its own answer there.
      body: JSON.stringify({
        optionIds: answer.optionIds,
        reason: answer.reason,
        content: answer.content,
      }),
    },
  )
}

/** Fetch one tool call's arguments or output. Returns null when retention has
 *  swept it, which is an ordinary outcome to render rather than an error. */
export async function getToolPayload(
  wsId: string,
  id: string,
  toolId: string,
  side: 'request' | 'result',
  signal?: AbortSignal,
): Promise<string | null> {
  const url =
    `${chatBase(wsId)}/${encodeURIComponent(id)}` +
    `/activity/${encodeURIComponent(toolId)}/payload?side=${side}`
  const response = await fetch(`${API_BASE}${url}`, { signal })
  if (response.status === 404) return null
  if (!response.ok) throw new Error(`tool payload: ${response.status}`)
  return response.text()
}

/** Read the provider's own report of context, cost and rate limits.
 *
 *  Null means the provider has not reported — which is NOT the same as reporting
 *  zero, and the caller must draw no gauge rather than an empty one. */
export async function getChatTelemetry(
  wsId: string,
  id: string,
  signal?: AbortSignal,
): Promise<AgentTelemetry | null> {
  const raw = await apiFetch<AgentTelemetry | null>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/telemetry`,
    { signal },
    { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 },
  )
  return raw ?? null
}

/** Ask Crowbar to restart the same interactive provider TUI with a completed
 *  prompt. `clientRequestId` is stable across retries. */
export async function submitAgentPrompt(
  wsId: string,
  id: string,
  text: string,
  clientRequestId: string,
): Promise<AgentPromptResult> {
  return apiFetch<AgentPromptResult>(`${chatBase(wsId)}/${encodeURIComponent(id)}/prompts`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text, clientRequestId }),
  })
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

// Map a wire provider into the store shape, defaulting the three enrichment flags
// so a backend row that omits them still reads sanely: never connected (install is
// never assumed), enabled (a provider with no stored preference is offered —
// spec §3.1), and with its tool surface on (the backend stores the NEGATIVE
// mcpDisabled, so an absent field there means enabled here — a default in the
// other direction would silently strip Crowbar's tools from an older daemon's
// providers). The backend always sends all three today; the defaults are
// belt-and-braces.
function mapProvider(p: AgentProvider): AgentProvider {
  return {
    id: p.id,
    displayName: p.displayName,
    icon: p.icon,
    connected: p.connected ?? false,
    enabled: p.enabled ?? true,
    mcpEnabled: p.mcpEnabled ?? true,
    // Both selection capabilities default OFF and both catalogues default EMPTY:
    // "this provider declares none" is what an older daemon's silence actually
    // means, and no-catalogue must render as no picker rather than an empty one.
    modelSelect: p.modelSelect ?? false,
    effortSelect: p.effortSelect ?? false,
    // Same direction and for the same reason: silence means the descriptor
    // declares no compaction gesture, and POST /compact answers 404 for it.
    compaction: p.compaction ?? false,
    // Opposite direction from every capability above: an omitted hasTerminal
    // describes an older daemon whose providers all had a real terminal, not a
    // provider that lacks one — see the field's own doc comment.
    hasTerminal: p.hasTerminal ?? true,
    hotswap: p.hotswap ?? false,
    models: p.models ?? [],
    efforts: p.efforts ?? {},
    permissionLevels: p.permissionLevels ?? [],
  }
}

export async function listProviders(wsId: string): Promise<AgentProvider[]> {
  const raw = await apiFetch<AgentProvider[]>(`${chatBase(wsId)}/providers`)
  return (raw ?? []).map(mapProvider)
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

/** Override ONE chat's own approval level, independent of the global default
 *  above. Write-only — there is no GET for a single chat's level, so a caller
 *  has nothing to reconcile against and must not pretend otherwise. */
export async function setChatPermissionLevel(
  wsId: string,
  chatId: string,
  level: PermissionLevel,
): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(chatId)}/permission-level`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ level }),
  })
}

// ── Writes ──────────────────────────────────────────────────────────
/**
 * Start a chat, optionally UNDER `parentId` — a chat (making the new one a
 * thread of it) or a folder ("new chat in here"). '' is the workspace root.
 *
 * Placement belongs on the create and not in a second call after it. The daemon
 * mints the chat, writes the edge, and only then starts the runner — so a thread
 * has its lineage before its first CLI is spawned and gets its ancestors
 * injected on that very first session. Creating and then placing meant the first
 * turn — the one right after the user asked for a thread — ran with no lineage
 * at all and the agent opened as a stranger. It also left a window in which a
 * failed second call stranded the new chat at the root.
 *
 * A folder INSIDE a chat still yields a thread of that chat: folders carry no
 * turns, so lineage steps straight through them.
 */
export async function createChat(wsId: string, provider: string, parentId = ''): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    // The repo-scoped mount binds no :wsId (Task 17), so wsId has to travel in
    // the body — the backend's Create falls back to body.workspaceId exactly
    // when the URL carries none. Omitting it anchors the chat to "", and a
    // top-level "" chat has no ancestor to resolve a cwd workspace from, so its
    // runner spawn 404s (agentchat: not found) even though the chat minted.
    body: JSON.stringify({ provider, parentId, workspaceId: wsId }),
  })
  return res.id
}

/**
 * Create a chat AND mint a fresh workspace for it to own, atomically (model
 * spec §4.1's "own worktree" create; backend Task 7,
 * `chat/handlers/chats.go`'s `Create`). Used by the sidebar's "create
 * workspace" affordance (space-content-actions.ts's `handleCreate`) in place
 * of the old two-step `postWorkspace` (a bare, chat-less workspace) plus a
 * later, separate chat create — this is ONE request, so the row that results
 * already has its first conversation running.
 *
 * Unlike `createChat`, this names NO workspace at all — there is none to name
 * yet — so it is built straight off project+repo rather than through
 * `chatBase`'s workspace-scope resolution, which requires an EXISTING
 * workspace to resolve a scope from. `workspaceId` is omitted from the body
 * entirely: the backend's `ownWorktree` only takes effect when the request
 * names no workspace, and an omitted key is the same "" its Go struct would
 * bind anyway.
 */
export async function createChatWithOwnWorktree(
  projectId: string,
  repoId: string,
  provider: string,
  parentId = '',
): Promise<string> {
  const p = encodeURIComponent(projectId)
  const r = encodeURIComponent(repoId)
  const res = await apiFetch<{ id: string }>(`/v0/projects/${p}/repos/${r}/chats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider, parentId, ownWorktree: true }),
  })
  return res.id
}

// switchProvider quits the chat's current vendor CLI, hands off the accumulated
// context, and starts `provider` as a NEW RUNNER on the same chat. Returns that
// runner's id — the chat is unchanged, the process is not.
export async function switchProvider(wsId: string, id: string, provider: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}/${encodeURIComponent(id)}/switch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ provider }),
  })
  return res.id
}

// resumeChat revives a DORMANT chat — one no runner points at, because its CLI
// exited or died with the daemon (agent PTYs are never persisted, so a restart
// always takes them). The backend brings the chat's last provider back into its
// own native session, so the conversation continues exactly where it left off.
//
// Returns the id of the RUNNER now on the chat. A chat that is still live is a
// no-op that hands back the runner already there, so this can never end up with
// two CLIs on one conversation.
export async function resumeChat(wsId: string, id: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}/${encodeURIComponent(id)}/resume`, {
    method: 'POST',
  })
  return res.id
}

// compactChat asks the chat's CLI to compact its OWN context. Crowbar cannot compact
// anything itself — the context belongs to the provider — so this triggers whatever
// gesture the provider declares: claude has no API for it and gets its /compact slash
// command, an API-transport provider gets the equivalent call.
//
// A provider that declares no gesture answers 404. Gate the control on the provider's
// `compaction` capability rather than letting the user press something that cannot work.
export async function compactChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/compact`, {
    method: 'POST',
  })
}

// stopChat gracefully terminates the chat's live vendor CLI and leaves the chat
// DORMANT and resumable — the counterpart of resumeChat. It is what closing a
// chat TAB calls: the agent process stops, but the chat entry and its bound
// conversation are KEPT, so reopening the tab revives the real conversation
// through the same resume path. This is NOT deleteChat: the chat is preserved.
// A chat whose CLI is already gone is a backend no-op.
export async function stopChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/stop`, {
    method: 'POST',
  })
}

// switchToTerminal hands the chat's live turn over to its provider's OWN native
// view — idle-only, for a provider whose `hotswap` capability is false (its
// terminal is not already live the way a hotswap provider's always is). Returns
// the new terminal session id to point the terminal surface at. Rejects
// (409) while a turn is in flight, and (422) for a provider with no native
// view to show at all — gate the control on `hasTerminal`/`hotswap` rather
// than letting the user press something that cannot work.
export async function switchToTerminal(wsId: string, id: string): Promise<string> {
  const res = await apiFetch<{ id: string }>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/switch-to-terminal`,
    { method: 'POST' },
  )
  return res.id
}

// switchToNative reverses switchToTerminal: the native-view PTY is torn down
// and the chat's api connection is re-established over the same session. A
// chat with nothing attached is a backend no-op.
export async function switchToNative(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/switch-to-native`, {
    method: 'POST',
  })
}

/**
 * Write the chat's sticky model + effort selection.
 *
 * BOTH halves travel on EVERY write, and '' clears one back to the provider's own
 * default. That is the endpoint's contract, not a convenience: which effort levels
 * exist is a property of the MODEL, so a partial write could store a pair that was
 * never jointly valid. Callers that change the model must therefore decide what
 * happens to the effort in the same call — see AgentModelPicker, which clears an
 * effort the new model does not declare rather than sending it on.
 *
 * 202 with no body on success. A value outside the provider's declared catalogue
 * is a 400 and a chat with no provider yet is a 422; both arrive as an ApiError
 * carrying that status, and both are the caller's to SHOW rather than swallow.
 */
export async function setChatSelection(
  wsId: string,
  id: string,
  model: string,
  effort: string,
): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/selection`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ model, effort }),
  })
}

export async function renameChat(wsId: string, id: string, title: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/rename`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

// Fills a bubble's empty workspace slot (model spec §4.2): a new worktree
// forked from its resolved fork parent, with its current provider respawned
// there. No request body — the server resolves everything from the chat's
// own id — and the response DTO is discarded here for the same reason every
// other perform* action discards it (row-actions.ts): the daemon's own
// broadcast/reseed is what the row actually repaints from.
export async function promoteChat(wsId: string, id: string): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}/promote`, {
    method: 'POST',
  })
}

// Deleting a chat CASCADES to the threads hanging off it — a thread exists to
// continue the conversation above it, and leaving it behind strands it reading a
// context that no longer exists.
//
// `init` exists for one caller: the removal tray's pagehide flush, which sends
// with `keepalive` so an eight-second undo window that ends in a reload still
// posts the delete the user already walked away from.
export async function deleteChat(wsId: string, id: string, init?: RequestInit): Promise<void> {
  await apiFetch<unknown>(`${chatBase(wsId)}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    ...init,
  })
  clearPersistedPromptQueue(wsId, id)
}

// ── The chats tree: folders, and where a row sits ───────────────────
//
// Parentage and folders are SERVER state, per workspace, exactly as the
// sidebar's folders are. An earlier client-persisted version of this was
// rejected: a thread reads its parent's turns, so which row a chat hangs off
// decides what an agent sees, and that cannot live in a browser key that another
// window — or another machine — never learns about.
//
// Every write answers with the rows a dense renumber MOVED as well as the row
// asked about. Apply both halves or the level paints in its old order.

export async function listChatFolders(wsId: string): Promise<AgentChatFolder[]> {
  const raw = await apiFetch<AgentChatFolderWire[]>(`${chatBase(wsId)}/folders`)
  return (raw ?? []).map(mapFolder)
}

export async function createChatFolder(
  wsId: string,
  name: string,
  parentId: string,
): Promise<{ folder: AgentChatFolder; shifted: AgentChatFolder[] }> {
  const raw = await apiFetch<{ folder: AgentChatFolderWire } & Partial<FolderShift>>(
    `${chatBase(wsId)}/folders`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, parentId }),
    },
  )
  return { folder: mapFolder(raw.folder), shifted: mapShift(raw) }
}

/** Rename, re-parent or reorder a folder. Only the named fields are written. */
export async function updateChatFolder(
  wsId: string,
  folderId: string,
  patch: { name?: string; parentId?: string; order?: number },
): Promise<{ folder: AgentChatFolder; shifted: AgentChatFolder[] }> {
  const raw = await apiFetch<{ folder: AgentChatFolderWire } & Partial<FolderShift>>(
    `${chatBase(wsId)}/folders/${encodeURIComponent(folderId)}`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    },
  )
  return { folder: mapFolder(raw.folder), shifted: mapShift(raw) }
}

// Deleting a folder PROMOTES its children to the folder's own parent — a folder
// holds no conversation, so the chats outlive it. That is deliberately unlike
// deleting a CHAT, which takes its threads with it.
export async function deleteChatFolder(
  wsId: string,
  folderId: string,
  init?: RequestInit,
): Promise<AgentChatFolder[]> {
  const raw = await apiFetch<Partial<FolderShift> | null>(
    `${chatBase(wsId)}/folders/${encodeURIComponent(folderId)}`,
    { method: 'DELETE', ...init },
  )
  return mapShift(raw)
}

/** Move a chat: under `parentId` (a chat, a folder, or '' for the root) at `order`. */
export async function setChatPlacement(
  wsId: string,
  chatId: string,
  patch: { parentId?: string; order?: number },
): Promise<{ chat: AgentChat; shifted: AgentChatFolder[] }> {
  const raw = await apiFetch<{ chat: AgentChat } & Partial<FolderShift>>(
    `${chatBase(wsId)}/${encodeURIComponent(chatId)}/placement`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    },
  )
  return { chat: mapChat(raw.chat), shifted: mapShift(raw) }
}
