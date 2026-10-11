import { Button } from '@/components/ui/button'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { hostFailureMessage } from '@/features/hosts/lib/host-failure-notice'

interface ChatLoadFailureProps {
  error: Error
  onRetry: () => void
  onOpenTerminal: () => void
}

/** What a chat whose messages could not be loaded shows instead: when its host is why, which
 *  host and what it did; a local failure reads as it always has. */
export function ChatLoadFailure({ error, onRetry, onOpenTerminal }: ChatLoadFailureProps) {
  const hostNotice = hostFailureMessage(error)
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center text-sm">
      <p>Couldn’t load this chat’s messages.</p>
      {hostNotice && <p className="text-muted-foreground">{hostNotice}</p>}
      <div className="flex gap-2">
        <Button size="sm" variant="secondary" onClick={onRetry}>
          Retry
        </Button>
        <Button size="sm" variant="ghost" onClick={onOpenTerminal}>
          <TerminalIcon /> Terminal
        </Button>
      </div>
    </div>
  )
}
