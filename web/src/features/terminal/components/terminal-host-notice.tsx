import { hostDotLabel, hostIsUnreachable } from '@/features/hosts/components/host-status-dot'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { LOCAL_HOST } from '@/features/hosts/types'
import { useResolvedHost } from '@/lib/host-resolve'
import { useHostUnavailable, type HostUnavailable } from '@/lib/host-unavailability'

function Notice({ title, detail }: { title: string; detail?: string }) {
  return (
    <div
      role="status"
      className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-1 bg-background/80 p-8 text-center"
    >
      <span className="text-sm font-medium text-foreground">{title}</span>
      {detail && <span className="text-xs text-muted-foreground">{detail}</span>}
    </div>
  )
}

const UNRESOLVED: Record<HostUnavailable | 'waiting', string> = {
  waiting: 'Waiting for this terminal’s host…',
  'host-unavailable': 'The host this terminal runs on is not available right now',
  'project-missing': 'Its project is no longer in your list',
}

function UnresolvedHostNotice({ wsId }: { wsId: string }) {
  const reason = useHostUnavailable({ wsId })
  return <Notice title={UNRESOLVED[reason ?? 'waiting']} />
}

function RemoteHostNotice({ hostId }: { hostId: string }) {
  const { name = 'This host', state } = useHostView(hostId)
  if (hostIsUnreachable(state)) {
    return <Notice title={`${name} is not available right now`} detail={hostDotLabel(state)} />
  }
  // It answers, but refuses its streams: this terminal's among them, or the xterm stays blank.
  if (state === 'no-live-stream') {
    return <Notice title={`${name}: no live stream — this terminal can’t attach`} />
  }
  // Up: the terminal is attaching, which says nothing about the host.
  if (state !== 'unknown' && state !== 'probing') return null
  return <Notice title={`Waiting for ${name}…`} />
}

/** What a detached terminal says while its host is the reason: not known yet, or not
 *  answering. Static, over any stale output; a Local terminal never shows it. */
export function TerminalHostNotice({ wsId }: { wsId: string }) {
  const hostId = useResolvedHost({ wsId })
  if (hostId === null) return <UnresolvedHostNotice wsId={wsId} />
  if (hostId === LOCAL_HOST.id) return null
  return <RemoteHostNotice hostId={hostId} />
}
