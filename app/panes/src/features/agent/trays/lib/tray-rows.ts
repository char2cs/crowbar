export interface TrayRow {
  id: string
  kind: 'subagent' | 'shell' | 'monitor'
  label: string
  detail: string
  hint?: string
  status: 'running' | 'done' | 'failed' | 'stopped'
  stopping?: boolean
  shellId?: string
  startedAt: string
  endedAt?: string
}

type TrayEntry = { type: 'label'; id: string; text: string } | { type: 'row'; row: TrayRow }

export interface TrayWindow {
  entries: TrayEntry[]
  more: { hidden: number; running: number } | null
  canCollapse: boolean
}

const MAX_ROWS = 4

const GROUPS: { kind: TrayRow['kind']; label: string }[] = [
  { kind: 'subagent', label: 'Subagents' },
  { kind: 'shell', label: 'Shells' },
  { kind: 'monitor', label: 'Monitors' },
]
const GROUP_LABELS = Object.fromEntries(GROUPS.map((g) => [g.kind, g.label])) as Record<
  TrayRow['kind'],
  string
>
const rank = (row: TrayRow) => GROUPS.findIndex((g) => g.kind === row.kind)

export function windowRows(rows: TrayRow[], expanded: boolean): TrayWindow {
  const ordered = [...rows].sort((a, b) => rank(a) - rank(b))
  const folds = ordered.length > MAX_ROWS && !expanded
  const shown = folds ? ordered.slice(0, MAX_ROWS - 1) : ordered
  const hidden = ordered.slice(shown.length)
  const labelled = new Set(shown.map((row) => row.kind)).size > 1
  const entries: TrayEntry[] = []
  let last: TrayRow['kind'] | null = null
  for (const row of shown) {
    if (labelled && row.kind !== last) {
      entries.push({ type: 'label', id: `label:${row.kind}`, text: GROUP_LABELS[row.kind] })
    }
    last = row.kind
    entries.push({ type: 'row', row })
  }
  return {
    entries,
    more: folds
      ? { hidden: hidden.length, running: hidden.filter((row) => row.status === 'running').length }
      : null,
    canCollapse: expanded && ordered.length > MAX_ROWS,
  }
}

const NOUNS: Record<TrayRow['kind'], [string, string]> = {
  subagent: ['subagent', 'subagents'],
  shell: ['shell', 'shells'],
  monitor: ['monitor', 'monitors'],
}

export function summarizeRows(rows: TrayRow[]): string {
  return GROUPS.flatMap(({ kind }) => {
    const n = rows.filter((row) => row.kind === kind).length
    return n > 0 ? [`${n} ${NOUNS[kind][n === 1 ? 0 : 1]}`] : []
  }).join(' · ')
}
