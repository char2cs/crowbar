import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import type { MarkerKind, PlanItem, RowBodies, ToolCallBody } from '@/features/agent/api/chat-rows'
import type { ToolFamily } from '@/features/agent/transcript/lib/tool-family'
import type { CallRound } from '@/features/agent/stores/lib/fold-rounds'

interface ItemBase {
  id: string
  /** The seq of the item's first row; 0 while it is known only from deltas. */
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
  /** The turn reply this message joins, as its first row naming one said. */
  reply?: string
}

export type ToolStatus = 'running' | 'ok' | 'error' | 'declined' | 'interrupted' | 'abandoned'

/** A tool's status once `body` lands on it. */
export function toolStatus(body: RowBodies['tool_call'], prev: ToolStatus | undefined): ToolStatus {
  if (body.phase === 'post') return body.status === 'error' ? 'error' : 'ok'
  if (body.phase === 'fail') {
    if (body.interrupted) return 'interrupted'
    return body.status === 'declined' ? 'declined' : 'error'
  }
  return prev ?? 'running'
}

/** The status of an upserted step that leaves the plan (ledger/plan.go). */
const STEP_REMOVED = 'removed'

/** `items` with each of `next` replacing the item of its id, or appended. */
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
  /** Decided by the call's first row; it never changes. */
  family: ToolFamily
  /** The ask gating this call, while it is pending. */
  askId?: string
  /** How that ask was decided, once it was. */
  askNote?: string
}

/** `choice.id` is the ask id; it stays pending until its ask_resolved row. */
export interface AskItem extends ItemBase {
  kind: 'ask'
  scope: string
  choice: AgentChoice
  /** What a permission is about to touch: the ask's own, else its tool call's. */
  toolTarget: string
  toolId?: string
  /** The gated call's input; with `toolInputTruncated`, its leading text. */
  toolInput?: unknown
  toolInputTruncated?: boolean
  mcpServer?: string
}

export type MarkerName =
  MarkerKind | 'turn_interrupted' | 'turn_failed' | 'session_lost' | 'runner_exited'

export interface MarkerItem extends ItemBase {
  kind: 'marker'
  marker: MarkerName
  detail?: string
  reason?: string
}

export type SubagentStatus =
  'starting' | 'running' | 'done' | 'failed' | 'interrupted' | 'abandoned'

/** A subagent from spawn to result, keyed by the call that spawned it (or its
 *  scope when no call did); its rows are `scoped[scope]` once bound. */
export interface SubagentItem extends ItemBase {
  kind: 'subagent'
  spawnToolId?: string
  scope?: string
  title: string
  type: string
  status: SubagentStatus
  result?: string
  usage?: { tokens?: number; toolUses?: number; durationMs?: number }
  /** The parent is waiting on, or closing, this subagent. */
  parentWaiting: boolean
  startedAt: string
  endedAt?: string
}

export interface ReasoningItem extends ItemBase {
  kind: 'reasoning'
  text: string
  startedAt: string
  endedAt: string
}

/** The main plan as its turn left it. */
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
  | ReasoningItem
  | PlanSnapshotItem

export type PromptFact =
  /** `launch` when a CLI launch carries it: that launch's dividers read above it. */
  | { state: 'pending'; seq: number; launch?: true }
  | { state: 'delivered'; turnId: string }
  /** `builtin` when the CLI ran it itself (a command) and no turn took it. */
  | { state: 'settled'; builtin?: true }
  | { state: 'refused'; reason: string }
  /** `turnId` when its turn spoke before the retract ended it: the row stays. */
  | { state: 'retracted'; turnId?: string }

export interface TurnFacts {
  running: { turnId: string; requestId?: string; startedAt: string; seq: number } | null
  /** requestId → its state, from rows. */
  prompts: Record<string, PromptFact>
  compacting: boolean
  openAsks: string[]
  /** Subagent cards still starting or running. */
  liveSubagents: string[]
  /** scope → the card bound to it. */
  subagentScopes: Record<string, string>
  /** A subagent that announced its scope while a spawn it may be is unbound. */
  pendingScopes: Record<string, { type?: string; title?: string }>
  /** By scope, latest. */
  plan: Record<string, PlanItem[]>
  lastExit: { runnerId: string; reason: string; exitCode?: number; seq: number } | null
  /** The turn Stop was pressed on; it reads as stopping only while it runs. */
  stopping: string | null
  /** Turn ids that produced text or a tool. */
  output: Record<string, true>
  /** Turns a retract took back before they said anything, hidden until output follows. */
  traceless: Record<string, { requestId: string; marker: string }>
  /** turnId → the effort its turn_ended reported. */
  efforts: Record<string, string>
  /** turnId → the provider its turn_started named. */
  providers: Record<string, string>
  /** turnId → its main-scope calls since its last message. */
  calls: Record<string, CallRound>
}

export interface ChatFeed {
  status: 'connecting' | 'live' | 'error'
  error?: string
  epoch: string
  /** The last applied row. */
  seq: number
  /** Main-scope item ids by the seq of their first row. */
  order: string[]
  /** Ids known only from deltas, after the order. */
  liveTail: string[]
  items: Record<string, TranscriptItem>
  /** `<stream>:<delta id>` → live text; each stream numbers its own frames. */
  live: Record<string, string>
  liveN: Record<string, number>
  /** The live key of the newest main-scope reasoning stream. */
  liveReasoning: string | null
  /** Item ids of rows in a subagent scope, by scope. */
  scoped: Record<string, string[]>
  /** Main-scope ids a turn end can still settle, by turn id ('' for none). */
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
export const subagentItemId = (key: string) => `s:${key}`
export const planItemId = (turnId: string) => `l:${turnId}`
export const liveKey = (stream: string, id: string) => `${stream}:${id}`
