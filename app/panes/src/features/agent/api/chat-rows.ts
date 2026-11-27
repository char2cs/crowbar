export interface LedgerRow<K extends RowKind = RowKind> {
  chatId: string
  seq: number
  kind: K
  turnId?: string
  scope?: string
  at: string
  body: RowBodies[K]
  trimmed?: string[]
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
    delivery?: 'restart' | 'steer' | 'api' | 'type'
    runnerId?: string
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
  runner_exited: { runnerId: string; reason: string; exitCode: number; detail?: string }
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
  shell: {
    phase: 'start' | 'note' | 'stopping' | 'end'
    shellId: string
    kind?: 'shell' | 'monitor'
    command?: string
    label?: string
    cwd?: string
    ref?: string
    persistent?: boolean
    timeoutMs?: number
    status?: 'finished' | 'failed' | 'stopped'
    exitCode?: number
    tail?: string
    summary?: string
  }
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
  id: string
  turnId: string
  scope?: string
  stream: 'answer' | 'reasoning' | 'tool_output' | 'diff'
  n: number
  text: string
  replace?: boolean
}

export type FeedFrame =
  | { type: 'row'; row: LedgerRow; from?: number }
  | { type: 'ready'; epoch: string }
  | { type: 'delta'; delta: FeedDelta }
  | { type: 'resync'; seq?: number }
