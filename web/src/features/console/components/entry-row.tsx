import type { ConsoleEntry } from '@/features/console/lib/entries'
import { LogRow } from './log-row'

const NOTE =
  'text-muted-foreground min-h-[18px] px-4 pl-[114px] font-mono text-xs leading-[18px] whitespace-pre-wrap [overflow-wrap:anywhere]'

interface EntryRowProps {
  entry: ConsoleEntry
  expanded: boolean
  onToggle: (id: number) => void
}

/** Any line of the console: a daemon log, text the daemon sent that did not decode, or a note. */
export function EntryRow({ entry, expanded, onToggle }: EntryRowProps) {
  switch (entry.kind) {
    case 'log':
      return <LogRow id={entry.id} record={entry.record} expanded={expanded} onToggle={onToggle} />
    case 'raw':
      return (
        <div data-slot="console-raw" className={NOTE}>
          {entry.text}
        </div>
      )
    case 'restarted':
      return (
        <div data-slot="console-note" className={NOTE}>
          Daemon restarted
        </div>
      )
  }
}
