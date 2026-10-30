import { mapChoice } from '@/features/agent/api/agent-chat-activity'
import type { LedgerRow, PlanItem, RowBodies, RowKind } from '@/features/agent/api/chat-rows'
import { fuller, textOf } from '@/features/agent/stores/lib/fold-live'
import {
  applyPromptSettled,
  applyUserMessage,
  retractTurn,
  setPrompt,
} from '@/features/agent/stores/lib/fold-prompts'
import {
  askItemId,
  markerItemId,
  textItemId,
  toolItemId,
  type AskItem,
  type ChatFeed,
  type MarkerName,
  type TextItem,
  type ToolItem,
  type ToolStatus,
  type TranscriptItem,
  type TurnFacts,
} from '@/features/agent/stores/lib/transcript-items'

const emptyFacts: TurnFacts = {
  running: null,
  prompts: {},
  compacting: false,
  openAsks: [],
  liveSubagents: {},
  plan: {},
  lastExit: null,
  sessionLost: null,
  output: {},
  ended: 0,
  efforts: {},
  providers: {},
}

export function emptyFeed(): ChatFeed {
  return {
    status: 'connecting',
    epoch: '',
    seq: 0,
    order: [],
    liveTail: [],
    items: {},
    live: {},
    liveN: {},
    liveReasoning: null,
    scoped: {},
    unsettled: {},
    facts: emptyFacts,
  }
}

// Markers a reader sees as a pill; the rest change facts only.
const PILL_MARKERS = new Set([
  'compaction_ended',
  'provider_switched',
  'model_changed',
  'effort_changed',
])

type FactRecord = 'prompts' | 'output' | 'efforts' | 'providers'

/** Edits to a feed over one or many rows. Each part is copied on its first
 *  write, so what the rows leave alone keeps its identity, and a replay of
 *  n rows copies each part once instead of n times. */
export class FeedFold {
  private items: Record<string, TranscriptItem> | null = null
  private order: string[] | null = null
  private liveTail: string[] | null = null
  private scoped: Record<string, string[]> | null = null
  private open: Record<string, string[]> | null = null
  private factsCopy: TurnFacts | null = null
  private readonly ownedScopes = new Set<string>()
  private readonly ownedFacts = new Set<FactRecord>()
  private seq: number

  constructor(private readonly base: ChatFeed) {
    this.seq = base.seq
  }

  /** Folds one row; a row at or below the folded seq is already applied. */
  apply(row: LedgerRow) {
    if (row.seq <= this.seq) return
    const apply = appliers[row.kind] as Apply<RowKind> | undefined
    apply?.(this, row)
    this.seq = row.seq
  }

  item<T extends TranscriptItem>(id: string): T | undefined {
    return (this.items ?? this.base.items)[id] as T | undefined
  }

  put(item: TranscriptItem) {
    this.items ??= { ...this.base.items }
    this.items[item.id] = item
  }

  get facts(): TurnFacts {
    return this.factsCopy ?? this.base.facts
  }

  set<K extends keyof TurnFacts>(key: K, value: TurnFacts[K]) {
    if (this.facts[key] === value) return
    this.factsCopy ??= { ...this.base.facts }
    this.factsCopy[key] = value
  }

  /** A per-turn fact record this fold may write in place. */
  record<K extends FactRecord>(key: K): TurnFacts[K] {
    if (!this.ownedFacts.has(key)) {
      this.ownedFacts.add(key)
      this.set(key, { ...this.facts[key] })
    }
    return this.facts[key]
  }

  /** Places a new item: main scope in the order, a subagent's under its scope. */
  place(id: string, scope: string) {
    if (scope) {
      this.scoped ??= { ...this.base.scoped }
      if (!this.ownedScopes.has(scope)) {
        this.ownedScopes.add(scope)
        this.scoped[scope] = [...(this.scoped[scope] ?? [])]
      }
      this.scoped[scope].push(id)
      return
    }
    const tail = this.liveTail ?? this.base.liveTail
    if (tail.includes(id)) this.liveTail = tail.filter((other) => other !== id)
    this.order ??= [...this.base.order]
    this.order.push(id)
    const item = this.item(id)
    if (item && isUnsettled(item))
      this.setOpen(item.turnId ?? '', [...this.openIn(item.turnId ?? ''), id])
  }

  unplace(id: string) {
    const order = this.order ?? this.base.order
    if (order.includes(id)) this.order = order.filter((other) => other !== id)
  }

  /** Main-scope ids a turn end can still change, by turn ('' for none). */
  openIn(turn: string): string[] {
    return (this.open ?? this.base.unsettled)[turn] ?? []
  }

  get openTurns(): string[] {
    return Object.keys(this.open ?? this.base.unsettled)
  }

  setOpen(turn: string, ids: string[]) {
    this.open ??= { ...this.base.unsettled }
    if (ids.length > 0) this.open[turn] = ids
    else delete this.open[turn]
  }

  finish(): ChatFeed {
    return {
      ...this.base,
      seq: this.seq,
      items: this.items ?? this.base.items,
      order: this.order ?? this.base.order,
      liveTail: this.liveTail ?? this.base.liveTail,
      scoped: this.scoped ?? this.base.scoped,
      unsettled: this.open ?? this.base.unsettled,
      facts: this.factsCopy ?? this.base.facts,
    }
  }
}

const isUnsettled = (item: TranscriptItem) =>
  (item.kind === 'text' && !item.settled) || (item.kind === 'tool' && item.status === 'running')

type Apply<K extends RowKind> = (fold: FeedFold, row: LedgerRow<K>) => void

const markOutput = (fold: FeedFold, row: LedgerRow) => {
  if (row.scope || !row.turnId || fold.facts.output[row.turnId]) return
  fold.record('output')[row.turnId] = true
}

function marker(
  fold: FeedFold,
  row: LedgerRow,
  name: MarkerName,
  detail?: string,
  reason?: string,
) {
  const id = markerItemId(row.seq)
  fold.put({
    kind: 'marker',
    id,
    seq: row.seq,
    turnId: row.turnId,
    at: row.at,
    marker: name,
    detail,
    reason,
  })
  fold.place(id, '')
}

/** Settles the texts of `turnId` (every turn when null) and abandons its running tools. */
function endTurn(fold: FeedFold, turnId: string | null) {
  for (const turn of turnId === null ? fold.openTurns : [turnId]) {
    const still: string[] = []
    for (const id of fold.openIn(turn)) {
      const item = fold.item(id)
      if (!item || !isUnsettled(item)) continue
      if (item.kind === 'text') fold.put({ ...item, settled: true })
      else if (item.kind === 'tool' && turnId !== null) fold.put({ ...item, status: 'abandoned' })
      else still.push(id)
    }
    fold.setOpen(turn, still)
  }
}

const applyTurnStarted: Apply<'turn_started'> = (fold, row) => {
  if (row.scope || !row.turnId) return
  const { requestId, provider } = row.body
  fold.set('running', { turnId: row.turnId, requestId, startedAt: row.at, seq: row.seq })
  if (requestId) setPrompt(fold, requestId, { state: 'delivered', turnId: row.turnId })
  if (provider) fold.record('providers')[row.turnId] = provider
}

function closeRunning(fold: FeedFold, turnId: string | undefined) {
  if (fold.facts.running && fold.facts.running.turnId === turnId) fold.set('running', null)
}

const applyTurnEnded: Apply<'turn_ended'> = (fold, row) => {
  if (row.scope || !row.turnId) return
  const traceless = retractTurn(fold, row)
  closeRunning(fold, row.turnId)
  fold.set('ended', fold.facts.ended + 1)
  if (row.body.effort) fold.record('efforts')[row.turnId] = row.body.effort
  endTurn(fold, row.turnId)
  if (row.body.status === 'interrupted' && !traceless)
    marker(fold, row, 'turn_interrupted', undefined, row.body.reason)
}

const applyTurnFailed: Apply<'turn_failed'> = (fold, row) => {
  if (row.scope || !row.turnId) return
  closeRunning(fold, row.turnId)
  endTurn(fold, row.turnId)
  marker(fold, row, 'turn_failed', row.body.detail, row.body.reason)
}

const applyRunnerExited: Apply<'runner_exited'> = (fold, row) => {
  const { runnerId, reason, exitCode } = row.body
  fold.set('lastExit', { runnerId, reason, exitCode, seq: row.seq })
  endTurn(fold, null)
}

const applySessionLost: Apply<'session_lost'> = (fold, row) => {
  fold.set('sessionLost', { provider: row.body.provider, reason: row.body.reason, seq: row.seq })
}

const applyMarker: Apply<'marker'> = (fold, row) => {
  const { kind, detail } = row.body
  if (kind === 'compaction_started') fold.set('compacting', true)
  if (kind === 'compaction_ended') fold.set('compacting', false)
  if (!row.scope && PILL_MARKERS.has(kind)) marker(fold, row, kind, detail)
}

/** A message naming the reply its turn closed on is shown as that reply. */
function joinedReply(fold: FeedFold, row: LedgerRow<'assistant_text'>): TextItem | undefined {
  const { reply, messageId } = row.body
  if (!reply || reply === messageId) return undefined
  return fold.item<TextItem>(textItemId(row.turnId, reply))
}

const applyAssistantText: Apply<'assistant_text'> = (fold, row) => {
  const scope = row.scope ?? ''
  const { messageId, index, final, text, phase } = row.body
  const id = textItemId(row.turnId, messageId)
  const known = fold.item<TextItem>(id)
  const item: TextItem = known
    ? { ...known, chunks: { ...known.chunks } }
    : {
        kind: 'text',
        id,
        seq: row.seq,
        turnId: row.turnId,
        at: row.at,
        messageId,
        chunks: {},
        text: '',
        settled: false,
        phase: '',
      }
  if (index === -1) {
    item.complete = fuller(item.complete ?? '', text)
    item.phase = phase || item.phase
  } else if (!(index in item.chunks)) {
    item.chunks[index] = text
    if (final) item.finalIndex = index
  }
  const folded = textOf(item)
  item.text = folded.text
  item.settled = item.settled || folded.settled
  if (known && known.seq === 0) item.seq = row.seq
  fold.put(item)
  markOutput(fold, row)
  const target = joinedReply(fold, row)
  if (target && (target.text.startsWith(item.text) || item.text.startsWith(target.text))) {
    if (item.settled) fold.put({ ...target, text: fuller(target.text, item.text) })
    return
  }
  if (!known || known.seq === 0) fold.place(id, scope)
}

function toolStatus(body: RowBodies['tool_call'], prev: ToolStatus | undefined): ToolStatus {
  if (body.phase === 'post') return body.status === 'error' ? 'error' : 'ok'
  if (body.phase === 'fail') {
    if (body.interrupted) return 'interrupted'
    return body.status === 'declined' ? 'declined' : 'error'
  }
  return prev ?? 'running'
}

const applyToolCall: Apply<'tool_call'> = (fold, row) => {
  const scope = row.scope ?? ''
  const id = toolItemId(row.body.toolId)
  const known = fold.item<ToolItem>(id)
  const body = known ? { ...known.body, ...row.body } : row.body
  const status = toolStatus(row.body, known?.status)
  fold.put(
    known
      ? { ...known, body, status }
      : {
          kind: 'tool',
          id,
          seq: row.seq,
          turnId: row.turnId,
          at: row.at,
          toolId: row.body.toolId,
          scope,
          body,
          status,
        },
  )
  markOutput(fold, row)
  if (!known) fold.place(id, scope)
}

const applyAsk: Apply<'ask'> = (fold, row) => {
  const scope = row.scope ?? ''
  const body = row.body
  const id = askItemId(body.askId)
  const choice = mapChoice({
    id: body.askId,
    turnId: row.turnId ?? '',
    seq: row.seq,
    kind: body.kind ?? body.event,
    toolName: body.toolName,
    title: body.title,
    question: body.question,
    mode: body.mode,
    multi: body.multi,
    options: body.options ?? [],
    questions: body.questions,
    schema: body.schema === undefined ? undefined : JSON.stringify(body.schema),
    pending: true,
    answerable: body.answerable,
    at: row.at,
  })
  const known = fold.item<AskItem>(id)
  const tool = body.toolId ? fold.item<ToolItem>(toolItemId(body.toolId)) : undefined
  fold.put({
    kind: 'ask',
    id,
    seq: known?.seq ?? row.seq,
    turnId: row.turnId,
    at: row.at,
    scope,
    choice,
    toolTarget: body.toolTarget || tool?.body.target || '',
    toolId: body.toolId,
  })
  if (!fold.facts.openAsks.includes(id)) fold.set('openAsks', [...fold.facts.openAsks, id])
  if (!known) fold.place(id, scope)
}

const applyAskResolved: Apply<'ask_resolved'> = (fold, row) => {
  const id = askItemId(row.body.askId)
  const known = fold.item<AskItem>(id)
  if (known) {
    const choice = {
      ...known.choice,
      pending: false,
      resolution: row.body.resolution,
      resolvedAt: row.at,
      answeredOptionIds: row.body.optionIds,
    }
    fold.put({ ...known, choice })
    // A denied tool never runs, so no post or fail row will settle it.
    const picked = new Set(row.body.optionIds)
    const denied =
      row.body.resolution === 'refused' ||
      known.choice.options.some((o) => o.kind === 'deny' && picked.has(o.id))
    const tool = denied && known.toolId ? fold.item<ToolItem>(toolItemId(known.toolId)) : undefined
    if (tool?.status === 'running') fold.put({ ...tool, status: 'declined' })
  }
  const openAsks = fold.facts.openAsks.filter((other) => other !== id)
  if (openAsks.length !== fold.facts.openAsks.length) fold.set('openAsks', openAsks)
}

const applySubagent: Apply<'subagent'> = (fold, row) => {
  const { phase, spawnToolId, type, title } = row.body
  const spawnKey = spawnToolId ? `s:${spawnToolId}` : ''
  const key = row.scope || spawnKey
  if (!key) return
  const live = { ...fold.facts.liveSubagents }
  if (phase === 'start') live[key] = { scope: key, type, title, startedAt: row.at }
  if (phase === 'bound' && row.scope && spawnKey && live[spawnKey]) {
    live[row.scope] = { ...live[spawnKey], scope: row.scope }
    delete live[spawnKey]
  }
  if (phase === 'stop' || phase === 'result') delete live[key]
  fold.set('liveSubagents', live)
}

function upsert(items: PlanItem[], next: PlanItem[]): PlanItem[] {
  const out = [...items]
  for (const item of next) {
    const at = item.id ? out.findIndex((other) => other.id === item.id) : -1
    if (at >= 0) out[at] = item
    else out.push(item)
  }
  return out
}

const applyPlan: Apply<'plan'> = (fold, row) => {
  const scope = row.scope ?? ''
  const { op, items } = row.body
  const current = fold.facts.plan[scope] ?? []
  fold.set('plan', {
    ...fold.facts.plan,
    [scope]: op === 'replace' ? items : upsert(current, items),
  })
}

const ignore: Apply<RowKind> = () => {}

const appliers: { [K in RowKind]: Apply<K> } = {
  user_message: applyUserMessage,
  turn_started: applyTurnStarted,
  prompt_settled: applyPromptSettled,
  turn_ended: applyTurnEnded,
  turn_failed: applyTurnFailed,
  session_bound: ignore,
  session_lost: applySessionLost,
  runner_started: ignore,
  runner_exited: applyRunnerExited,
  assistant_text: applyAssistantText,
  reasoning: ignore,
  marker: applyMarker,
  tool_call: applyToolCall,
  ask: applyAsk,
  ask_resolved: applyAskResolved,
  subagent: applySubagent,
  plan: applyPlan,
}

/** Folds one row; a row at or below the feed's seq is already applied. */
export function applyRow(feed: ChatFeed, row: LedgerRow): ChatFeed {
  if (row.seq <= feed.seq) return feed
  const fold = new FeedFold(feed)
  fold.apply(row)
  return fold.finish()
}
