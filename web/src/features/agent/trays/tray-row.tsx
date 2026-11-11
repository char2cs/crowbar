import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { useTextClock } from '@/features/agent/activity/lib/text-clock'
import type { TrayRow } from '@/features/agent/trays/lib/tray-rows'

/** One subagent or shell: a spinner while it runs, what it is, what it is doing, how long. */
export function TrayRowView({ row }: { row: TrayRow }) {
  const running = row.status === 'running'
  const clockRef = useTextClock(running ? row.startedAt : undefined)
  return (
    <li data-kind={row.kind} data-status={row.status}>
      {running ? <FlickerSpinner className="size-4 text-primary" /> : <i className="tray-gap" />}
      <i className="tray-who">{row.label}</i>
      <i className="tray-what">{row.detail}</i>
      <i className="tray-clock" data-testid="agent-tray-clock" ref={clockRef} />
    </li>
  )
}
