import type { ToolItem, TranscriptItem } from '@/features/agent/stores/lib/transcript-items'

/** One transcript line: an item, or a run of read-only calls keyed by its first. */
export interface DisplayRow {
  id: string
  members?: string[]
}

const FOLDS = new Set<ToolItem['family']>(['read', 'search', 'fetch'])
const HIDDEN = new Set<ToolItem['family']>(['meta', 'plan'])

const foldable = (item: TranscriptItem | undefined): item is ToolItem =>
  item?.kind === 'tool' && FOLDS.has(item.family)

/** The order as drawn: meta and plan calls hidden, consecutive read-only calls
 *  of one turn folded into a run whose key stays as it grows. Depends only on
 *  each item's kind, family and turn, which never change. */
export function displayRows(order: string[], items: Record<string, TranscriptItem>): DisplayRow[] {
  const rows: DisplayRow[] = []
  let run: (DisplayRow & { members: string[]; turn?: string }) | null = null
  for (const id of order) {
    const item = items[id]
    if (item?.kind === 'tool' && HIDDEN.has(item.family)) continue
    if (!foldable(item)) {
      run = null
      rows.push({ id })
      continue
    }
    if (run && run.turn === item.turnId) {
      run.members.push(id)
      continue
    }
    run = { id: `g:${id}`, members: [id], turn: item.turnId }
    rows.push(run)
  }
  return rows.map((row) => (row.members ? { id: row.id, members: row.members } : row))
}

const NOUNS: Partial<Record<ToolItem['family'], [string, string, string, string]>> = {
  read: ['reading', 'read', 'file', 'files'],
  search: ['searching', 'searched', 'pattern', 'patterns'],
  fetch: ['fetching', 'fetched', 'page', 'pages'],
}

/** "Reading 2 files, searching 1 pattern…" while live, "Read 2 files, …" after. */
export function runSentence(items: ToolItem[], live: boolean): string {
  const counts = new Map<ToolItem['family'], number>()
  for (const item of items) counts.set(item.family, (counts.get(item.family) ?? 0) + 1)
  const parts = [...counts].map(([family, count]) => {
    const [ing, ed, one, many] = NOUNS[family] ?? ['calling', 'called', 'tool', 'tools']
    return `${live ? ing : ed} ${count} ${count === 1 ? one : many}`
  })
  const sentence = parts.join(', ')
  return `${sentence.charAt(0).toUpperCase()}${sentence.slice(1)}${live ? '…' : ''}`
}
