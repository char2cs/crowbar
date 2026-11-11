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
  /** A shell or monitor the agent was asked to end and has not yet. */
  stopping?: boolean
  /** The provider's id of a shell or monitor, which a stop names. */
  shellId?: string
  startedAt: string
  /** When it ended, so an ended row keeps how long it ran. */
  endedAt?: string
}

type TrayEntry = { type: 'label'; id: string; text: string } | { type: 'row'; row: TrayRow }

export interface TrayWindow {
  entries: TrayEntry[]
  /** The grouped row standing in for the rows folded away. */
  more: { hidden: number; running: number } | null
  /** Everything is showing and there was more than fits. */
  canCollapse: boolean
}

/** How long a row has run, the way the board writes it: `58s`, `1m 42s`, `1h 05m`. */
export function formatTrayClock(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  if (safe < 60) return `${safe}s`
  const pad = (n: number) => String(n).padStart(2, '0')
  if (safe < 3600) return `${Math.floor(safe / 60)}m ${pad(safe % 60)}s`
  return `${Math.floor(safe / 3600)}h ${pad(Math.floor((safe % 3600) / 60))}m`
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

const NOUNS: Record<TrayRow['kind'], [string, string]> = {
  subagent: ['subagent', 'subagents'],
  shell: ['shell', 'shells'],
  monitor: ['monitor', 'monitors'],
}

/** What the tray holds in a line, for the phone's folded row: `3 subagents · 2 shells · 1 monitor`. */
export function summarizeRows(rows: TrayRow[]): string {
  return GROUPS.map(({ kind }) => {
    const n = rows.filter((row) => row.kind === kind).length
    return n > 0 ? `${n} ${NOUNS[kind][n === 1 ? 0 : 1]}` : ''
  })
    .filter(Boolean)
    .join(' · ')
}
