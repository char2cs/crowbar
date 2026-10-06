import { memo } from 'react'
import {
  formatLogTime,
  jsonLines,
  type FieldKind,
  type LogLevel,
  type LogRecord,
} from '@/features/console/lib/frames'
import { cn } from '@/utils/cn'

const LEVEL_CLASS: Record<LogLevel, string> = {
  debug: 'text-muted-foreground',
  info: 'text-info',
  warn: 'text-warning',
  error: 'text-destructive',
}

// Only warnings and errors colour their message: a problem is the one line that is not plain.
const MESSAGE_CLASS: Record<LogLevel, string> = {
  debug: '',
  info: '',
  warn: 'text-warning',
  error: 'text-destructive',
}

const FIELD_CLASS: Record<FieldKind, string> = {
  text: 'text-code-foreground',
  number: 'text-info',
  duration: 'text-info',
  bool: 'text-success',
  path: 'text-code-foreground underline decoration-border underline-offset-2',
  error: 'text-destructive',
}

interface LogRowProps {
  id: number
  record: LogRecord
  expanded: boolean
  onToggle: (id: number) => void
}

/**
 * One daemon log line: time, level, then the message and every other key as
 * `key=value`. The component is not a column of its own (most lines have none);
 * it is in the record JSON, one click away.
 */
export const LogRow = memo(function LogRow({ id, record, expanded, onToggle }: LogRowProps) {
  return (
    <div data-slot="log-row" data-level={record.level}>
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => onToggle(id)}
        className={cn(
          'text-code-foreground hover:bg-muted focus-visible:bg-muted grid w-full cursor-pointer grid-cols-[88px_44px_minmax(0,1fr)] gap-x-2.5 px-4 py-[3px] text-left font-mono text-xs leading-[18px] outline-none',
          expanded && 'bg-muted',
        )}
      >
        <span className="text-muted-foreground">{formatLogTime(record.iso)}</span>
        <span className={cn('font-semibold tracking-[0.04em]', LEVEL_CLASS[record.level])}>
          {record.level.toUpperCase()}
        </span>
        <span className="[overflow-wrap:anywhere]">
          <span className={MESSAGE_CLASS[record.level]}>{record.msg}</span>
          {record.fields.map((field) => (
            <span key={field.key} className="ml-2.5 inline-block">
              <span className="text-muted-foreground after:content-['=']">{field.key}</span>
              <span className={FIELD_CLASS[field.kind]}>{field.value}</span>
            </span>
          ))}
        </span>
      </button>
      {expanded && <JsonView record={record} />}
    </div>
  )
})

function JsonView({ record }: { record: LogRecord }) {
  const lines = jsonLines(record)
  return (
    <div
      data-slot="log-json"
      className="bg-muted text-muted-foreground py-1.5 pr-4 pl-[168px] font-mono text-xs leading-[18px]"
    >
      <div>{'{'}</div>
      {lines.map((line, i) => (
        <div key={line.key} className="pl-4">
          <span>&quot;{line.key}&quot;</span>
          <span>: </span>
          <span className={FIELD_CLASS[line.kind]}>{line.value}</span>
          {i < lines.length - 1 && <span>,</span>}
        </div>
      ))}
      <div>{'}'}</div>
    </div>
  )
}
