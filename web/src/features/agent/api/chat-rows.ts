// The feed's wire shapes: a mirror of the daemon's ledger rows
// (api/internal/domain/ledger) and feed frames (usecases/chat/feed.go).

export interface LedgerRow<K extends RowKind = RowKind> {
  chatId: string
  seq: number
  kind: K
  turnId?: string
  /** '' or absent: the main agent. */
  scope?: string
  at: string
  body: RowBodies[K]
}

export type RowKind = keyof RowBodies

export type MarkerKind =
  | 'session_changed'
  | 'session_conflict'
  | 'dead_letter'
  | 'compaction_started'
  | 'compaction_ended'
  | 'stopped'
  | 'provider_switched'
  | 'model_changed'
  | 'effort_changed'
  | 'notification'

export interface PlanItem {
  id?: string
  text: string
  detail?: string
  status?: string
}

export interface ToolCallBody {
  toolId: string
  phase: 'pre' | 'post' | 'fail'
  name?: string
  kind?: string
  title?: string
  target?: string
  locations?: { path: string; line?: number }[]
  inputRef?: string
  resultRef?: string
  status?: string
  error?: string
  durationMs?: number
  diff?: string
  mcpServer?: string
  mcpTool?: string
  exitCode?: number
  interrupted?: boolean
  inputKey?: string
  spawnsScope?: string
  addressesScope?: string
}

interface AskBody {
  askId: string
  event: string
  kind?: string
  promptId?: string
  toolId?: string
  toolName?: string
  title?: string
  question?: string
  mode?: string
  multi?: boolean
  options?: { id: string; kind: string; label?: string; description?: string }[]
  questions?: {
    id: string
    title?: string
    text?: string
    multi?: boolean
    options: { id: string; kind: string; label?: string; description?: string }[]
  }[]
  schema?: unknown
  detail?: string
  toolTarget?: string
  toolInput?: unknown
  toolInputTruncated?: boolean
  mcpServer?: string
  inputKey?: string
  answerable: boolean
}

export interface RowBodies {
  user_message: {
    requestId?: string
    text: string
    origin: 'crowbar' | 'provider' | 'harness' | 'note'
    delivery?: 'restart' | 'steer' | 'api'
    runnerId?: string
    dispatch?: string
  }
  turn_started: {
    requestId?: string
    providerTurnId?: string
    runnerId?: string
    provider?: string
    sessionId?: string
    persists?: 'turn_start' | 'turn_end'
  }
  prompt_settled: { requestId: string; consumed: boolean; reason: string }
  turn_ended: {
    status: 'done' | 'interrupted'
    reason?: string
    runnerId?: string
    effort?: string
    diff?: string
  }
  turn_failed: { reason: string; detail?: string }
  session_bound: {
    provider: string
    sessionId: string
    cause: 'assigned' | 'announced' | 'fresh'
    previous?: string
  }
  session_lost: { provider: string; sessionId: string; reason: string }
  runner_started: {
    runnerId: string
    provider: string
    terminalSession: string
    launchModel: string
    launchEffort: string
    launchPermissionLevel: string
    resumeSessionId: string
  }
  runner_exited: { runnerId: string; reason: string; exitCode: number }
  /** index -1: the message's complete text; reply: the reply this message joins. */
  assistant_text: {
    messageId: string
    index: number
    final?: boolean
    text: string
    phase?: '' | 'commentary'
    reply?: string
  }
  reasoning: { messageId?: string; text: string }
  marker: { kind: MarkerKind; detail?: string; providerTurnId?: string }
  tool_call: ToolCallBody
  ask: AskBody
  ask_resolved: {
    askId: string
    resolution: 'answered' | 'proceeded' | 'refused' | 'abandoned'
    optionIds?: string[]
  }
  subagent: {
    phase: 'start' | 'stop' | 'bound' | 'result'
    spawnToolId?: string
    type?: string
    title?: string
    status?: string
    result?: string
    usage?: { tokens?: number; toolUses?: number; durationMs?: number }
  }
  plan: { op: 'replace' | 'upsert'; items: PlanItem[] }
}

export interface FeedDelta {
  /** The message id (answer, reasoning) or the tool id (tool_output, diff). */
  id: string
  turnId: string
  scope?: string
  stream: 'answer' | 'reasoning' | 'tool_output' | 'diff'
  /** 1-based position in its stream. */
  n: number
  /** The new suffix, or everything so far when `replace`. */
  text: string
  replace?: boolean
}

// Go `omitempty`: an absent seq is 0.
export type FeedFrame =
  | { type: 'row'; row: LedgerRow }
  | { type: 'ready'; epoch: string; seq?: number }
  | { type: 'delta'; delta: FeedDelta }
  | { type: 'resync'; seq?: number }
  | { type: 'deleted' }
