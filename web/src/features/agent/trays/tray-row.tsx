import { EyeIcon, TerminalWindowIcon } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { useTextClock } from '@/features/agent/activity/lib/text-clock'
import type { TrayRow } from '@/features/agent/trays/lib/tray-rows'

/** One subagent, shell or monitor: a spinner while it runs, what it is, what it is doing or
 *  how it ended, how long it has run. */
export function TrayRowView({ row }: { row: TrayRow }) {
  const running = row.status === 'running'
  const clockRef = useTextClock(row.startedAt)
  return (
    <li data-kind={row.kind} data-status={row.status} title={row.hint}>
      {running ? <FlickerSpinner className="size-4 text-primary" /> : <i className="tray-gap" />}
      {row.kind === 'shell' && <TerminalWindowIcon size={14} aria-label="Shell" />}
      {row.kind === 'monitor' && <EyeIcon size={14} aria-label="Monitor" />}
      <i className="tray-who">{row.label}</i>
      <i className="tray-what">{row.detail}</i>
      {running && <i className="tray-clock" data-testid="agent-tray-clock" ref={clockRef} />}
    </li>
  )
}
