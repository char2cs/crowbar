import type { ItemMap } from '@/features/agent/stores/lib/item-map'
import type { ToolItem, TranscriptItem } from '@/features/agent/stores/lib/transcript-items'

export interface DisplayRow {
  id: string
  members?: string[]
}

const FOLDS = new Set<ToolItem['family']>(['read', 'search', 'fetch'])
const HIDDEN = new Set<ToolItem['family']>(['meta', 'plan'])

function sameRow(a: DisplayRow | undefined, b: DisplayRow): boolean {
  if (a?.id !== b.id) return false
  const [x, y] = [a.members ?? [], b.members ?? []]
  return x.length === y.length && x.every((id, i) => id === y[i])
}

const asked = (item: ToolItem) => item.askId !== undefined || item.askNote !== undefined

const foldable = (item: TranscriptItem | undefined): item is ToolItem =>
  item?.kind === 'tool' && FOLDS.has(item.family)

export function displayRows(order: string[], items: ItemMap, prev?: DisplayRow[]): DisplayRow[] {
  const rows: DisplayRow[] = []
  let run: (DisplayRow & { members: string[]; turn?: string }) | null = null
  for (const id of order) {
    const item = items.get(id)
    if (item?.kind === 'tool' && HIDDEN.has(item.family) && !asked(item)) continue
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
  return rows.map((row, i) => {
    const next = row.members ? { id: row.id, members: row.members } : row
    return sameRow(prev?.[i], next) ? prev![i]! : next
  })
}

const NOUNS: Partial<Record<ToolItem['family'], [string, string, string, string]>> = {
  read: ['reading', 'read', 'file', 'files'],
  search: ['running', 'ran', 'search', 'searches'],
  fetch: ['fetching', 'fetched', 'page', 'pages'],
}

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
