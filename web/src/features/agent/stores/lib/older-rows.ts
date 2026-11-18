import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { emptyFeed, FeedFold } from '@/features/agent/stores/lib/fold-rows'
import type { ChatFeed, TurnFacts } from '@/features/agent/stores/lib/transcript-items'

const RECORDS = [
  ...['prompts', 'subagentScopes', 'pendingScopes', 'plan', 'output', 'traceless'],
  ...['efforts', 'providers', 'models', 'calls'],
] as const satisfies readonly (keyof TurnFacts)[]

/** What the older turns knew, under what the held ones say: their live state is the held feed's. */
function mergeFacts(held: TurnFacts, older: TurnFacts): TurnFacts {
  const facts = { ...held }
  for (const key of RECORDS) Object.assign(facts, { [key]: { ...older[key], ...held[key] } })
  facts.shells = [...new Set([...older.shells, ...held.shells])]
  return facts
}

/** `feed` with the whole turns of `rows` (the page that ends at its `from`) laid above, as folding
 *  the ledger from `from` would have left them: the held items and order are shared, not refolded. */
export function prependOlder(feed: ChatFeed, rows: LedgerRow[], from: number): ChatFeed {
  const fold = new FeedFold(emptyFeed())
  fold.startAt(from)
  for (const row of rows) fold.apply(row)
  const older = fold.finish()
  const items = feed.items.edit()
  for (const item of older.items.all()) if (!items.get(item.id)) items.set(item)
  const placed = new Set(feed.order)
  const scoped = { ...older.scoped, ...feed.scoped }
  return {
    ...feed,
    from,
    items: items.finish(),
    order: [...older.order.filter((id) => !placed.has(id)), ...feed.order],
    scoped,
    facts: mergeFacts(feed.facts, older.facts),
  }
}

/** `feed` with item `id` holding what `row`, the whole of the row its preview was cut from, says. */
export function fillFull(feed: ChatFeed, id: string, row: LedgerRow): ChatFeed {
  const item = feed.items.get(id)
  if (!item) return feed
  const body = row.body as Record<string, unknown>
  let next = item
  if (item.kind === 'tool') {
    next = { ...item, body: { ...item.body, diff: body.diff as string }, fullSeq: undefined }
  } else if (item.kind === 'reasoning') {
    next = { ...item, text: body.text as string, fullSeq: undefined }
  } else if (item.kind === 'subagent') {
    next = { ...item, result: body.result as string, fullSeq: undefined }
  }
  if (next === item) return feed
  const items = feed.items.edit()
  items.set(next)
  return { ...feed, items: items.finish() }
}
