import type { HostUnavailable } from '@/lib/host-unavailability'

const COPY: Record<HostUnavailable | 'waiting', { title: string; body?: string }> = {
  waiting: { title: 'Opening this chat…' },
  'host-unavailable': {
    title: 'Chat unavailable',
    body: 'The host this chat runs on is not available right now.',
  },
  'project-missing': {
    title: 'Chat unavailable',
    body: 'Its project is no longer in your list. You can close this tab.',
  },
}

/** What a chat pane says in place of the chat while its host is not known: still
 *  waiting (`reason` null), or why the wait is over. Static: nothing here animates. */
export function AgentChatUnavailable({ reason }: { reason: HostUnavailable | null }) {
  const { title, body } = COPY[reason ?? 'waiting']
  return (
    <div
      role="status"
      className="flex h-full w-full flex-col items-center justify-center gap-1 p-8 text-center"
    >
      <span className="text-sm font-medium text-foreground">{title}</span>
      {body && <span className="text-xs text-muted-foreground">{body}</span>}
    </div>
  )
}
