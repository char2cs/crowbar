import type { LedgerRow, RowKind } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'
import {
  planItemId,
  subagentItemId,
  type PlanSnapshotItem,
  type ReasoningItem,
  type SubagentItem,
  type SubagentStatus,
} from '@/features/agent/stores/lib/transcript-items'

// Mirrors the daemon's SubagentBook (domain/ledger/subagent.go): the same
// edges, so a card never reads a status the daemon would refuse.
const EDGES: Partial<Record<SubagentStatus, SubagentStatus[]>> = {
  starting: ['running', 'failed', 'abandoned', 'interrupted'],
  running: ['done', 'failed', 'interrupted'],
}

const live = (status: SubagentStatus) => status === 'starting' || status === 'running'

type Apply<K extends RowKind> = (fold: FeedFold, row: LedgerRow<K>) => void

function putCard(fold: FeedFold, card: SubagentItem) {
  fold.put(card)
  const ids = fold.facts.liveSubagents
  const listed = ids.includes(card.id)
  if (live(card.status) && !listed) fold.set('liveSubagents', [...ids, card.id])
  if (!live(card.status) && listed) {
    fold.set(
      'liveSubagents',
      ids.filter((id) => id !== card.id),
    )
  }
}

function moveTo(fold: FeedFold, card: SubagentItem, to: SubagentStatus, at: string) {
  if (!EDGES[card.status]?.includes(to)) return
  putCard(fold, { ...card, status: to, endedAt: live(to) ? card.endedAt : at })
}

function newCard(
  row: LedgerRow,
  key: string,
  status: SubagentStatus,
  fields: Partial<SubagentItem>,
): SubagentItem {
  return {
    kind: 'subagent',
    id: subagentItemId(key),
    seq: row.seq,
    turnId: row.turnId,
    at: row.at,
    title: '',
    type: '',
    status,
    parentWaiting: false,
    startedAt: row.at,
    ...fields,
  }
}

const cardOf = (fold: FeedFold, scope: string) => {
  const id = fold.facts.subagentScopes[scope]
  return id ? fold.item<SubagentItem>(id) : undefined
}

/** The card a spawning call opens, in the call's own place and scope. */
export function openSpawn(fold: FeedFold, row: LedgerRow<'tool_call'>) {
  const { toolId, title } = row.body
  const known = fold.item<SubagentItem>(subagentItemId(toolId))
  if (known) {
    if (!known.title && title) fold.put({ ...known, title })
    return
  }
  putCard(fold, newCard(row, toolId, 'starting', { spawnToolId: toolId, title: title ?? '' }))
  fold.place(subagentItemId(toolId), row.scope ?? '')
}

/** A call that only waits on or closes a subagent says so on its card. */
export function addressSubagent(fold: FeedFold, row: LedgerRow<'tool_call'>) {
  const card = cardOf(fold, row.body.addressesScope ?? '')
  const waiting = row.body.phase === 'pre'
  if (card && card.parentWaiting !== waiting) fold.put({ ...card, parentWaiting: waiting })
}

function dropLive(fold: FeedFold, id: string) {
  const ids = fold.facts.liveSubagents
  if (ids.includes(id))
    fold.set(
      'liveSubagents',
      ids.filter((other) => other !== id),
    )
}

function bind(fold: FeedFold, row: LedgerRow<'subagent'>, scope: string, spawnToolId: string) {
  const spawned = fold.item<SubagentItem>(subagentItemId(spawnToolId))
  const announced = cardOf(fold, scope)
  if (announced && announced.id !== subagentItemId(spawnToolId)) {
    // It announced itself first: its card stays, and takes the call's.
    fold.put({ ...announced, spawnToolId, title: announced.title || spawned?.title || '' })
    if (spawned) {
      fold.unplace(spawned.id)
      dropLive(fold, spawned.id)
    }
    return
  }
  const pending = fold.facts.pendingScopes[scope]
  if (pending) delete fold.record('pendingScopes')[scope]
  const base =
    spawned ?? newCard(row, spawnToolId, 'running', { spawnToolId, title: row.body.title ?? '' })
  const card: SubagentItem = {
    ...base,
    scope,
    type: base.type || row.body.type || pending?.type || '',
    title: base.title || pending?.title || '',
  }
  fold.record('subagentScopes')[scope] = card.id
  putCard(fold, card.status === 'starting' ? { ...card, status: 'running' } : card)
  if (!spawned) fold.place(card.id, '')
}

/** A subagent that names its own scope: a spawn still unbound may be it, so it
 *  waits for the bind; else it is a card of its own. */
function announce(fold: FeedFold, row: LedgerRow<'subagent'>, scope: string) {
  const { type, title } = row.body
  const known = cardOf(fold, scope)
  if (known) {
    fold.put({ ...known, type: known.type || type || '', title: known.title || title || '' })
    return
  }
  const unbound = fold.facts.liveSubagents.some(
    (id) => fold.item<SubagentItem>(id)?.status === 'starting',
  )
  if (unbound) {
    fold.record('pendingScopes')[scope] = { type, title }
    return
  }
  claim(fold, row, scope, { type, title })
}

function claim(
  fold: FeedFold,
  row: LedgerRow,
  scope: string,
  fields: { type?: string; title?: string },
) {
  const card = newCard(row, scope, 'running', {
    scope,
    type: fields.type ?? '',
    title: fields.title ?? '',
  })
  fold.record('subagentScopes')[scope] = card.id
  putCard(fold, card)
  fold.place(card.id, '')
}

function finish(fold: FeedFold, row: LedgerRow<'subagent'>) {
  const { status, result, usage, spawnToolId } = row.body
  const card = row.scope
    ? cardOf(fold, row.scope)
    : fold.item<SubagentItem>(subagentItemId(spawnToolId ?? ''))
  if (!card) return
  const to: SubagentStatus = status === 'failed' || status === 'interrupted' ? status : 'done'
  const moved = EDGES[card.status]?.includes(to)
  putCard(fold, {
    ...card,
    status: moved ? to : card.status,
    endedAt: moved ? row.at : card.endedAt,
    result: card.result || result,
    usage: card.usage ?? usage,
  })
}

export const applySubagent: Apply<'subagent'> = (fold, row) => {
  const { phase, spawnToolId } = row.body
  const scope = row.scope ?? ''
  if (phase === 'start' && !scope && spawnToolId) {
    const known = fold.item<SubagentItem>(subagentItemId(spawnToolId))
    if (!known) {
      putCard(
        fold,
        newCard(row, spawnToolId, 'starting', { spawnToolId, title: row.body.title ?? '' }),
      )
      fold.place(subagentItemId(spawnToolId), '')
    } else if (!known.title && row.body.title) fold.put({ ...known, title: row.body.title })
  } else if (phase === 'start' && scope) {
    if (spawnToolId) bind(fold, row, scope, spawnToolId)
    else announce(fold, row, scope)
  } else if (phase === 'bound' && scope && spawnToolId) {
    bind(fold, row, scope, spawnToolId)
  } else if (phase === 'stop' || phase === 'result') {
    finish(fold, row)
  }
}

/** A turn's end abandons the spawns it never bound; a scope still waiting on
 *  one is a subagent of its own after all. */
export function endSubagents(fold: FeedFold, row: LedgerRow) {
  for (const id of fold.facts.liveSubagents) {
    const card = fold.item<SubagentItem>(id)
    if (card?.status === 'starting' && card.turnId === row.turnId)
      moveTo(fold, card, 'abandoned', row.at)
  }
  for (const [scope, pending] of Object.entries(fold.facts.pendingScopes)) {
    delete fold.record('pendingScopes')[scope]
    claim(fold, row, scope, pending)
  }
}

/** The CLI that ran them is gone. */
export function interruptSubagents(fold: FeedFold, row: LedgerRow) {
  for (const id of fold.facts.liveSubagents) {
    const card = fold.item<SubagentItem>(id)
    if (card) moveTo(fold, card, 'interrupted', row.at)
  }
}

/** Recorded once at the turn's end, it reads above the turn's reply. */
export const applyReasoning: Apply<'reasoning'> = (fold, row) => {
  if (row.scope) return
  const { messageId, text } = row.body
  const id = messageId ? `r:${row.turnId ?? ''}:${messageId}` : `r:${row.seq}`
  const running = fold.facts.running
  const item: ReasoningItem = {
    kind: 'reasoning',
    id,
    seq: row.seq,
    turnId: row.turnId,
    at: row.at,
    text,
    startedAt: running && running.turnId === row.turnId ? running.startedAt : row.at,
    endedAt: row.at,
  }
  fold.put(item)
  if (!fold.placed(id)) fold.placeAbove(id, row.turnId ?? '')
}

/** The newest main plan of a turn, kept unplaced until the turn ends. */
export function notePlan(fold: FeedFold, row: LedgerRow<'plan'>) {
  if (row.scope || !row.turnId) return
  const id = planItemId(row.turnId)
  const snapshot: PlanSnapshotItem = {
    kind: 'plan',
    id,
    seq: fold.item(id)?.seq ?? row.seq,
    turnId: row.turnId,
    at: row.at,
    items: fold.facts.plan[''] ?? [],
  }
  fold.put(snapshot)
}

export function placePlan(fold: FeedFold, turnId: string) {
  const id = planItemId(turnId)
  if (fold.item(id) && !fold.placed(id)) fold.place(id, '')
}
