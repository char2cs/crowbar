import { Skull } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { confirmAndKillAgent } from '@/features/agent/lib/kill-agent'
import { useActiveSessionList, viewersLabel, type ActiveSession } from '@/lib/store/active-sessions'

const SECONDARY = 'ui-font ui-text-sm truncate text-muted-foreground'

function SessionRow({ session }: { session: ActiveSession }) {
  return (
    <div
      data-testid="active-session-row"
      className="flex items-center gap-3 rounded-lg px-1 py-2 transition-colors hover:bg-muted/50"
    >
      <div className="min-w-0 flex-1">
        <div className="ui-font ui-text-sm truncate text-foreground">{session.title}</div>
        <div className={SECONDARY}>{viewersLabel(session.attached)}</div>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-label={`Kill the agent on ${session.title}`}
        className="text-muted-foreground hover:text-destructive"
        onClick={() => void confirmAndKillAgent(session)}
      >
        <Skull />
        Kill
      </Button>
    </div>
  )
}

/** Settings → Active sessions: every agent running right now, however it was left. */
export function ActiveSessionsSettings() {
  const sessions = useActiveSessionList()

  return (
    <div className="space-y-4">
      <div className="mb-2 px-1 py-1.5">
        <h3 className="font-heading text-[32px] text-foreground">Active sessions</h3>
        <p className="ui-font ui-text-sm text-muted-foreground">
          Closing a chat only detaches from it; its agent keeps running. Kill one here to end it.
        </p>
      </div>

      {sessions.length === 0 ? (
        <p className="ui-font ui-text-sm px-1 text-muted-foreground">No agents are running.</p>
      ) : (
        <div className="space-y-1">
          {sessions.map((session) => (
            <SessionRow key={session.chatId} session={session} />
          ))}
        </div>
      )}
    </div>
  )
}
