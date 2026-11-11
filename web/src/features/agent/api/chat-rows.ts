// The chat scope's wire shapes: a mirror of the daemon's ledger rows and
// deltas (api/internal/domain/ledger), and the frames the chat store folds.

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
  | 'warning'
  | 'legacy_unreadable'
  | 'from_record'

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
  /** What a finished call printed, when short enough to read inline. */
  output?: string
  mcpServer?: string
  mcpTool?: string
  exitCode?: number
  interrupted?: boolean
  spawnsScope?: string
  addressesScope?: string
}

interface AskBody {
  askId: string
  event: string
  kind?: string
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
  answerable: boolean
}

export interface RowBodies {
  user_message: {
    requestId?: string
    text: string
    origin: 'crowbar' | 'provider' | 'harness' | 'note'
    delivery?: 'restart' | 'steer' | 'api'
    runnerId?: string
    /** The refused prompt this one sends again. */
    retryOf?: string
  }
  turn_started: {
    requestId?: string
    runnerId?: string
    provider?: string
    sessionId?: string
  }
  prompt_settled: { requestId: string; consumed: boolean; reason: string }
  turn_ended: {
    status: 'done' | 'interrupted'
    reason?: string
    runnerId?: string
    effort?: string
    diff?: string
  }
  turn_failed: { reason: string; detail?: string; kind?: string }
  session_bound: { provider: string; sessionId: string; cause: string }
  session_lost: { provider: string; sessionId: string; reason: string; movedTo?: string }
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
  marker: { kind: MarkerKind; detail?: string; owner?: string; records?: string[] }
  tool_call: ToolCallBody
  ask: AskBody
  ask_resolved: {
    askId: string
    resolution: 'answered' | 'proceeded' | 'refused' | 'abandoned'
    optionIds?: string[]
  }
  subagent: {
    phase: 'start' | 'stop' | 'bound' | 'resume' | 'result'
    spawnToolId?: string
    type?: string
    title?: string
    status?: string
    result?: string
    usage?: { tokens?: number; toolUses?: number; durationMs?: number }
  }
  plan: { op: 'replace' | 'upsert'; items: PlanItem[] }
  /** A command the main agent left running in the background. */
  shell: {
    phase: 'start' | 'note' | 'end'
    shellId: string
    /** Absent for a plain shell. */
    kind?: 'shell' | 'monitor'
    command?: string
    label?: string
    cwd?: string
    /** The provider's handle on the process; not necessarily an OS pid. */
    ref?: string
    persistent?: boolean
    timeoutMs?: number
    status?: 'finished' | 'failed' | 'stopped'
    exitCode?: number
    /** The end of its output (an end) or its newest line (a note). */
    tail?: string
    summary?: string
  }
  /** The chat changing the folder its CLI runs in (api/internal/domain/ledger/move.go). */
  move: MoveBody
}

export interface MoveBody {
  phase: 'requested' | 'done' | 'failed' | 'cancelled'
  requestId: string
  to?: string
  parentId?: string
  order?: number
  label?: string
  branchOut?: boolean
  branch?: string
  by?: 'user' | 'agent'
  from?: string
  fromDir?: string
  toDir?: string
  base?: string
  repo?: string
  reason?: string
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
  /** `from`: on a window's first row, the seq it follows. */
  | { type: 'row'; row: LedgerRow; from?: number }
  | { type: 'ready'; epoch: string }
  | { type: 'delta'; delta: FeedDelta }
  | { type: 'resync'; seq?: number }
