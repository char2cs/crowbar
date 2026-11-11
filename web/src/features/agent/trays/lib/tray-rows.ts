/** One line of the bottom tray: work the agent started that is still being tracked. */
export interface TrayRow {
  id: string
  kind: 'subagent' | 'shell' | 'monitor'
  label: string
  /** What it is doing now, or how it ended. */
  detail: string
  /** Everything else known of it, for a tooltip. */
  hint?: string
  status: 'running' | 'done' | 'failed' | 'stopped'
  startedAt: string
}

type TrayEntry = { type: 'label'; id: string; text: string } | { type: 'row'; row: TrayRow }

export interface TrayWindow {
  entries: TrayEntry[]
  /** The grouped row standing in for the rows folded away. */
  more: { hidden: number; running: number } | null
  /** Everything is showing and there was more than fits. */
  canCollapse: boolean
}

/** The tray never grows past this many rows, its group row included. */
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

/** The rows to draw: subagents, then shells, then monitors, folded to fit unless `expanded`. */
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
