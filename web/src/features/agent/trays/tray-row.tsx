import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { useTextClock } from '@/features/agent/activity/lib/text-clock'
import { formatTrayClock, type TrayRow } from '@/features/agent/trays/lib/tray-rows'

function ranSeconds({ startedAt, endedAt }: TrayRow): number {
  const span = endedAt ? Date.parse(endedAt) - Date.parse(startedAt) : Number.NaN
  return Number.isNaN(span) ? 0 : Math.round(span / 1000)
}

/** One subagent, shell or monitor: a spinner while it runs, its name, what it is doing or how
 *  it ended, how long it has run. The name and the action share a line, or stack on a phone. */
export function TrayRowView({ row }: { row: TrayRow }) {
  const running = row.status === 'running'
  const ranFor = ranSeconds(row)
  const clockRef = useTextClock(row.startedAt, formatTrayClock)
  return (
    <li data-kind={row.kind} data-status={row.status} title={row.hint}>
      {running ? <FlickerSpinner className="size-4 text-primary" /> : <i className="tray-gap" />}
      <span className="tray-lines">
        <i className="tray-who">{row.label}</i>
        <i className="tray-what">{row.detail}</i>
      </span>
      {!running && ranFor > 0 && <i className="tray-clock">{formatTrayClock(ranFor)}</i>}
      {running && <i className="tray-clock" data-testid="agent-tray-clock" ref={clockRef} />}
    </li>
  )
}
