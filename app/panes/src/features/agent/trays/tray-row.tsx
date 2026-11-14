import { XIcon } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { formatClock, useTextClock } from '@/features/agent/activity/lib/text-clock'
import type { TrayRow } from '@/features/agent/trays/lib/tray-rows'

function ranSeconds({ startedAt, endedAt }: TrayRow): number {
  const span = endedAt ? Date.parse(endedAt) - Date.parse(startedAt) : Number.NaN
  return Number.isNaN(span) ? 0 : Math.round(span / 1000)
}

/** One subagent, shell or monitor: a spinner while it runs, its name, what it is doing or how
 *  it ended, how long it has run. The name and the action share a line, or stack on a phone. */
export function TrayRowView({
  row,
  onStop,
}: {
  row: TrayRow
  /** Asks the agent to end this shell or monitor; absent where it cannot be asked. */
  onStop?: (shellId: string) => void
}) {
  const running = row.status === 'running'
  const noun = row.kind === 'monitor' ? 'monitor' : 'shell'
  const { shellId } = row
  const stop = running && !row.stopping && shellId && onStop ? () => onStop(shellId) : undefined
  const ranFor = ranSeconds(row)
  const clockRef = useTextClock(row.startedAt, formatClock)
  return (
    <li data-kind={row.kind} data-status={row.status} title={row.hint}>
      {running ? <FlickerSpinner className="size-4 text-primary" /> : <i className="tray-gap" />}
      <span className="tray-lines">
        <i className="tray-who">{row.label}</i>
        <i className="tray-what">{row.detail}</i>
      </span>
      {!running && ranFor > 0 && <i className="tray-clock">{formatClock(ranFor)}</i>}
      {running && <i className="tray-clock" data-testid="agent-tray-clock" ref={clockRef} />}
      {stop && (
        <button
          type="button"
          className="tray-stop"
          aria-label={`End this ${noun}`}
          title={`End this ${noun}`}
          onClick={stop}
        >
          <XIcon size={12} />
        </button>
      )}
    </li>
  )
}
