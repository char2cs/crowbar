import type { FeedDelta, LedgerRow, RowKind } from '@/features/agent/api/chat-rows'
import { applyDelta, fuller, textOf } from '@/features/agent/stores/lib/fold-live'
import {
  applyPromptSettled,
  applyUserMessage,
  divider,
  retractTurn,
  reviveTurn,
  setPrompt,
} from '@/features/agent/stores/lib/fold-prompts'
import {
  applyAsk,
  applyAskResolved,
  applyToolCall,
  markOutput,
} from '@/features/agent/stores/lib/fold-tools'
import {
  applyReasoning,
  applySubagent,
  endSubagents,
  interruptSubagents,
  notePlan,
  endPlan,
} from '@/features/agent/stores/lib/fold-subagents'
import { placeMessage, settleRound } from '@/features/agent/stores/lib/fold-rounds'
import {
  liveKey,
  markerItemId,
  textItemId,
  upsertPlan,
  type ChatFeed,
  type MarkerName,
  type TextItem,
  type ToolStatus,
  type TranscriptItem,
  type TurnFacts,
} from '@/features/agent/stores/lib/transcript-items'

const emptyFacts: TurnFacts = {
  running: null,
  prompts: {},
  compacting: false,
  openAsks: [],
  liveSubagents: [],
  subagentScopes: {},
  pendingScopes: {},
  plan: {},
  lastExit: null,
  stopping: null,
  output: {},
  traceless: {},
  efforts: {},
  providers: {},
  calls: {},
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
type FactRecord = {
  [K in keyof TurnFacts]: TurnFacts[K] extends Record<string, unknown> ? K : never
}[keyof TurnFacts]

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

  /** Folds the row right after the folded seq. One at or below it is already
   *  applied; one past the next is dropped, since only a replay fills a hole. */
  apply(row: LedgerRow) {
    if (row.seq !== this.seq + 1) return
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

  /** Places a new item: main scope in the order, a subagent's under its scope.
   *  Answers whether it went above `before`, which must already be placed. */
  place(id: string, scope: string, before?: string) {
    if (scope) {
      this.scoped ??= { ...this.base.scoped }
      if (!this.ownedScopes.has(scope)) {
        this.ownedScopes.add(scope)
        this.scoped[scope] = [...(this.scoped[scope] ?? [])]
      }
      this.scoped[scope].push(id)
      return false
    }
    const tail = this.liveTail ?? this.base.liveTail
    if (tail.includes(id)) this.liveTail = tail.filter((other) => other !== id)
    const item = this.item(id)
    const turn = item?.turnId ?? ''
    if (this.facts.traceless[turn]) {
      if (item?.kind === 'marker') return false
      reviveTurn(this, turn)
    }
    this.order ??= [...this.base.order]
    // What a provider flushes after its turn ended still reads above that end.
    let at = before ? this.order.lastIndexOf(before) : -1
    const above = at >= 0
    if (!above) at = this.order.length
    while (item?.kind !== 'marker' && at > 0 && this.isEndOf(this.order[at - 1]!, turn)) at--
    this.order.splice(at, 0, id)
    const open = this.openIn(turn)
    if (item && isUnsettled(item) && !open.includes(id)) this.setOpen(turn, [...open, id])
    return above
  }

  private isEndOf(id: string, turn: string) {
    const item = this.item(id)
    return turn !== '' && item?.kind === 'marker' && item.turnId === turn
  }

  /** Moves `turn`'s streamed text into the order, above the turn's end, holding
   *  the words streamed so far: settled text no longer follows its stream. What
   *  streamed is output the turn showed, so a retract keeps it and its prompt. */
  placeLive(turn: string) {
    for (const id of this.liveTail ?? this.base.liveTail) {
      const item = this.item(id)
      if (item?.kind !== 'text' || item.turnId !== turn) continue
      const live = this.base.live[liveKey('answer', item.messageId)] ?? ''
      this.put({ ...item, text: fuller(item.text, live) })
      this.record('output')[turn] = true
      this.place(id, '')
    }
  }

  /** Places a main-scope item above the first output of `turn`. */
  placeAbove(id: string, turn: string) {
    this.order ??= [...this.base.order]
    const at = this.order.findIndex((other) => {
      const item = this.item(other)
      return item?.turnId === turn && item.kind !== 'user'
    })
    if (at < 0) this.place(id, '')
    else this.order.splice(at, 0, id)
  }

  placed(id: string) {
    return (this.order ?? this.base.order).includes(id)
  }

  unplace(id: string) {
    // From the end: what moves or goes is nearly always among the newest.
    const at = (this.order ?? this.base.order).lastIndexOf(id)
    if (at < 0) return
    this.order ??= [...this.base.order]
    this.order.splice(at, 1)
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

/** Settles the texts of `turnId` (every turn when null) and ends its running
 *  tools as `ended`: a stopped turn's were interrupted, any other's abandoned. */
function endTurn(fold: FeedFold, turnId: string | null, ended: ToolStatus = 'abandoned') {
  for (const turn of turnId === null ? fold.openTurns : [turnId]) {
    const still: string[] = []
    for (const id of fold.openIn(turn)) {
      const item = fold.item(id)
      if (!item || !isUnsettled(item)) continue
      if (item.kind === 'text') fold.put({ ...item, settled: true })
      else if (item.kind === 'tool' && turnId !== null) fold.put({ ...item, status: ended })
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
  // A provider may report an end Crowbar already recorded: one marker per turn.
  const ending = fold.facts.running?.turnId === row.turnId
  settleRound(fold, row.turnId)
  retractTurn(fold, row)
  closeRunning(fold, row.turnId)
  if (row.body.effort) fold.record('efforts')[row.turnId] = row.body.effort
  fold.placeLive(row.turnId)
  const interrupted = row.body.status === 'interrupted'
  endTurn(fold, row.turnId, interrupted ? 'interrupted' : 'abandoned')
  endPlan(fold, row.turnId)
  endSubagents(fold, row)
  if (interrupted && ending) marker(fold, row, 'turn_interrupted', undefined, row.body.reason)
}

const applyTurnFailed: Apply<'turn_failed'> = (fold, row) => {
  if (row.scope || !row.turnId) return
  settleRound(fold, row.turnId)
  closeRunning(fold, row.turnId)
  endTurn(fold, row.turnId)
  endPlan(fold, row.turnId)
  endSubagents(fold, row)
  marker(fold, row, 'turn_failed', row.body.detail, row.body.reason)
}

const applyRunnerExited: Apply<'runner_exited'> = (fold, row) => {
  const { runnerId, reason, exitCode } = row.body
  fold.set('lastExit', { runnerId, reason, exitCode, seq: row.seq })
  endTurn(fold, null)
  interruptSubagents(fold, row)
  if (!row.scope && ABNORMAL_EXITS.has(reason))
    marker(fold, row, 'runner_exited', undefined, reason)
}

/** Exits nobody asked for; the others (a stop, a switch, a retract) need no word. */
const ABNORMAL_EXITS = new Set(['exited', 'connection_lost', 'transport_overflow', 'spawn_failed'])

const applySessionLost: Apply<'session_lost'> = (fold, row) => {
  if (!row.scope) marker(fold, row, 'session_lost', row.body.provider, row.body.reason)
}

const applyMarker: Apply<'marker'> = (fold, row) => {
  const { kind, detail } = row.body
  if (kind === 'compaction_started') fold.set('compacting', true)
  if (kind === 'compaction_ended') fold.set('compacting', false)
  if (!row.scope) divider(fold, kind, () => marker(fold, row, kind, detail))
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
  // A delta made the item before any row; its first row dates it, as a replay would.
  if (known && known.seq === 0) {
    item.seq = row.seq
    item.at = row.at
  }
  fold.put(item)
  markOutput(fold, row)
  const target = joinedReply(fold, row)
  if (target && (target.text.startsWith(item.text) || item.text.startsWith(target.text))) {
    if (item.settled) fold.put({ ...target, text: fuller(target.text, item.text) })
    return
  }
  if (!known || (known.seq === 0 && !fold.placed(id))) placeMessage(fold, row, id)
}

const applyPlan: Apply<'plan'> = (fold, row) => {
  const scope = row.scope ?? ''
  const { op, items } = row.body
  const current = fold.facts.plan[scope] ?? []
  fold.set('plan', {
    ...fold.facts.plan,
    [scope]: op === 'replace' ? items : upsertPlan(current, items),
  })
  notePlan(fold, row)
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
  reasoning: applyReasoning,
  marker: applyMarker,
  tool_call: applyToolCall,
  ask: applyAsk,
  ask_resolved: applyAskResolved,
  subagent: applySubagent,
  plan: applyPlan,
}

/** Folds the row right after the feed's seq; any other is not applied. */
export function applyRow(feed: ChatFeed, row: LedgerRow): ChatFeed {
  if (row.seq !== feed.seq + 1) return feed
  const fold = new FeedFold(feed)
  fold.apply(row)
  return fold.finish()
}

/** Folds one delta; one for a turn a retract hid brings that turn back. */
export function applyLive(feed: ChatFeed, delta: FeedDelta): ChatFeed {
  const next = applyDelta(feed, delta)
  if (!next.facts.traceless[delta.turnId] || !next.liveTail.length) return next
  const fold = new FeedFold(next)
  fold.placeLive(delta.turnId)
  return fold.finish()
}
