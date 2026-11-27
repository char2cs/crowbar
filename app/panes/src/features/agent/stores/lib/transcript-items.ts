import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import type {
  MarkerKind,
  MoveBody,
  PlanItem,
  RowBodies,
  ToolCallBody,
} from '@/features/agent/api/chat-rows'
import type { ItemMap } from '@/features/agent/stores/lib/item-map'
import type { ToolFamily } from '@/features/agent/transcript/lib/tool-family'
import type { CallRound } from '@/features/agent/stores/lib/fold-rounds'

interface ItemBase {
  id: string
  seq: number
  turnId?: string
  at: string
}

export interface UserItem extends ItemBase {
  kind: 'user'
  text: string
  origin: 'crowbar' | 'provider' | 'harness' | 'note'
  requestId?: string
}

export interface TextItem extends ItemBase {
  kind: 'text'
  messageId: string
  chunks: Record<number, string>
  finalIndex?: number
  complete?: string
  text: string
  settled: boolean
  phase: '' | 'commentary'
  reply?: string
}

export type ToolStatus = 'running' | 'ok' | 'error' | 'declined' | 'interrupted' | 'abandoned'

export function toolStatus(body: RowBodies['tool_call'], prev: ToolStatus | undefined): ToolStatus {
  if (body.phase === 'post') return body.status === 'error' ? 'error' : 'ok'
  if (body.phase === 'fail') {
    if (body.interrupted) return 'interrupted'
    return body.status === 'declined' ? 'declined' : 'error'
  }
  return prev ?? 'running'
}

const STEP_REMOVED = 'removed'

export function upsertPlan(items: PlanItem[], next: PlanItem[]): PlanItem[] {
  let out = [...items]
  for (const item of next) {
    const at = item.id ? out.findIndex((other) => other.id === item.id) : -1
    if (item.status === STEP_REMOVED) out = at >= 0 ? out.filter((_, i) => i !== at) : out
    else if (at >= 0) out[at] = item
    else out.push(item)
  }
  return out
}

export interface ToolItem extends ItemBase {
  kind: 'tool'
  toolId: string
  scope: string
  body: ToolCallBody
  status: ToolStatus
  family: ToolFamily
  askId?: string
  askNote?: string
  fullSeq?: number
}

export interface AskItem extends ItemBase {
  kind: 'ask'
  scope: string
  choice: AgentChoice
  toolTarget: string
  toolId?: string
  toolInput?: unknown
  toolInputTruncated?: boolean
  mcpServer?: string
  family?: ToolFamily
  detail?: string
}

export type MarkerName =
  MarkerKind | 'turn_interrupted' | 'turn_failed' | 'session_lost' | 'runner_exited' | 'move'

export interface MarkerItem extends ItemBase {
  kind: 'marker'
  marker: MarkerName
  detail?: string
  reason?: string
  failureKind?: string
  chat?: string
  records?: string[]
  parts?: { marker: MarkerName; detail?: string }[]
  command?: string
  move?: MoveBody
}

export type SubagentStatus =
  'starting' | 'running' | 'done' | 'failed' | 'interrupted' | 'abandoned'

export interface SubagentItem extends ItemBase {
  kind: 'subagent'
  spawnToolId?: string
  scope?: string
  title: string
  type: string
  status: SubagentStatus
  result?: string
  usage?: { tokens?: number; toolUses?: number; durationMs?: number }
  fullSeq?: number
  parentWaiting: boolean
  startedAt: string
  endedAt?: string
}

type ShellStatus = 'running' | 'finished' | 'failed' | 'stopped'

export interface ShellItem extends ItemBase {
  kind: 'shell'
  shellId: string
  shellKind: 'shell' | 'monitor'
  command: string
  label: string
  cwd?: string
  ref?: string
  persistent?: boolean
  timeoutMs?: number
  status: ShellStatus
  stopping?: boolean
  exitCode?: number
  tail?: string
  summary?: string
  startedAt: string
  endedAt?: string
}

export interface ReasoningItem extends ItemBase {
  kind: 'reasoning'
  text: string
  fullSeq?: number
  startedAt: string
  endedAt: string
}

export interface PlanSnapshotItem extends ItemBase {
  kind: 'plan'
  items: PlanItem[]
}

export type TranscriptItem =
  | UserItem
  | TextItem
  | ToolItem
  | AskItem
  | MarkerItem
  | SubagentItem
  | ShellItem
  | ReasoningItem
  | PlanSnapshotItem

export type PromptFact =
  | { state: 'pending'; seq: number; launch?: true; steer?: true }
  | { state: 'delivered'; turnId: string }
  | { state: 'settled'; builtin?: true }
  | { state: 'refused'; reason: string }
  | { state: 'retried'; by: string }
  | { state: 'retracted'; turnId?: string }

export interface TurnFacts {
  running: { turnId: string; requestId?: string; startedAt: string; seq: number } | null
  prompts: Record<string, PromptFact>
  compacting: boolean
  openAsks: string[]
  decidedAsks: string[]
  liveSubagents: string[]
  shells: string[]
  subagentScopes: Record<string, string>
  pendingScopes: Record<string, { type?: string; title?: string }>
  plan: Record<string, PlanItem[]>
  stopping: string | null
  output: Record<string, true>
  traceless: Record<string, { requestId: string; marker: string }>
  efforts: Record<string, string>
  providers: Record<string, string>
  provider: string
  model: string
  models: Record<string, string>
  calls: Record<string, CallRound>
}

export interface ChatFeed {
  status: 'connecting' | 'live' | 'error'
  error?: string
  epoch: string
  seq: number
  from: number
  order: string[]
  liveTail: string[]
  items: ItemMap
  live: Record<string, string>
  liveN: Record<string, number>
  liveReasoning: string | null
  scoped: Record<string, string[]>
  unsettled: Record<string, string[]>
  facts: TurnFacts
}

export const userItemId = (requestId: string | undefined, seq: number) =>
  requestId ? `p:${requestId}` : `u:${seq}`
export const textItemId = (turnId: string | undefined, messageId: string) =>
  `m:${turnId ?? ''}:${messageId}`
export const toolItemId = (toolId: string) => `t:${toolId}`
export const askItemId = (askId: string) => `a:${askId}`
export const markerItemId = (seq: number) => `k:${seq}`
export const moveItemId = (requestId: string) => `m:${requestId}`
export const shellItemId = (shellId: string) => `h:${shellId}`
export const subagentItemId = (key: string) => `s:${key}`
export const planItemId = (turnId: string) => `l:${turnId}`
export const liveKey = (stream: string, id: string) => `${stream}:${id}`
