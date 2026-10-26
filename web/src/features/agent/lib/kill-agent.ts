import { killChat } from '@/features/agent/api/agent-api'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { toast } from '@/features/window/stores/toast-store'
import type { ActiveSession } from '@/lib/store/active-sessions'

/**
 * The one way a client ends a running agent: ask first, then force-kill. Any
 * paired client may do this to any chat, whoever else is attached. Resolves to
 * whether the kill went through.
 */
export async function confirmAndKillAgent(
  session: Pick<ActiveSession, 'chatId' | 'workspaceId' | 'title' | 'attached'>,
): Promise<boolean> {
  const viewers =
    session.attached > 0
      ? ` ${session.attached === 1 ? 'One client is' : `${session.attached} clients are`} attached to it.`
      : ''
  const confirmed = await primitiveConfirm(
    `It stops now, even mid-turn.${viewers} The conversation is kept and resumes when reopened.`,
    {
      title: `Kill the agent on “${session.title}”?`,
      confirmLabel: 'Kill agent',
      cancelLabel: 'Keep running',
      destructive: true,
    },
  )
  if (!confirmed) return false
  try {
    await killChat(session.workspaceId, session.chatId)
    return true
  } catch (err) {
    toast.error(
      `Couldn't kill the agent on ${session.title}`,
      err instanceof Error ? err.message : undefined,
    )
    return false
  }
}
