import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import type { MarkerKind, PlanItem, ToolCallBody } from '@/features/agent/api/chat-rows'

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
}

export type ToolStatus = 'running' | 'ok' | 'error' | 'declined' | 'interrupted' | 'abandoned'

export interface ToolItem extends ItemBase {
  kind: 'tool'
  toolId: string
  scope: string
  body: ToolCallBody
  status: ToolStatus
}

/** `choice.id` is the ask id; it stays pending until its ask_resolved row. */
export interface AskItem extends ItemBase {
  kind: 'ask'
  scope: string
  choice: AgentChoice
  /** What a permission is about to touch: the ask's own, else its tool call's. */
  toolTarget: string
  toolId?: string
}

export type MarkerName = MarkerKind | 'turn_interrupted' | 'turn_failed'

export interface MarkerItem extends ItemBase {
  kind: 'marker'
  marker: MarkerName
  detail?: string
  reason?: string
}

export type TranscriptItem = UserItem | TextItem | ToolItem | AskItem | MarkerItem

export type PromptFact =
  | { state: 'pending'; seq: number }
  | { state: 'delivered'; turnId: string }
  | { state: 'settled' }
  | { state: 'refused'; reason: string }
  | { state: 'retracted' }

export interface LiveSubagent {
  scope: string
  type?: string
  title?: string
  startedAt: string
}

export interface TurnFacts {
  running: { turnId: string; requestId?: string; startedAt: string; seq: number } | null
  /** requestId → its state, from rows. */
  prompts: Record<string, PromptFact>
  compacting: boolean
  openAsks: string[]
  liveSubagents: Record<string, LiveSubagent>
  /** By scope, latest. */
  plan: Record<string, PlanItem[]>
  lastExit: { runnerId: string; reason: string; exitCode?: number; seq: number } | null
  sessionLost: { provider: string; reason: string; seq: number } | null
  /** Turn ids that produced text or a tool. */
  output: Record<string, true>
  /** Turn ids a retract took back before they said anything: nothing of them shows. */
  traceless: Record<string, true>
  /** turnId → the effort its turn_ended reported. */
  efforts: Record<string, string>
  /** turnId → the provider its turn_started named. */
  providers: Record<string, string>
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
export const liveKey = (stream: string, id: string) => `${stream}:${id}`
