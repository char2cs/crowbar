import { killChat } from '@/features/agent/api/agent-chat-commands'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import { report } from '@/lib/errors/report'
import type { ActiveSession } from '@/core/entities/active-sessions'

type KillableAgent = Pick<ActiveSession, 'hostId' | 'chatId' | 'title' | 'viewers'>

export function killAgentConfirmation(session: KillableAgent): { title: string; message: string } {
  const viewers =
    session.viewers > 0
      ? ` ${session.viewers === 1 ? 'One client is' : `${session.viewers} clients are`} attached to it.`
      : ''
  return {
    title: `Kill the agent on “${session.title}”?`,
    message: `It stops now, even mid-turn.${viewers} The conversation is kept and resumes when reopened.`,
  }
}

export async function killAgent(session: KillableAgent): Promise<boolean> {
  try {
    await killChat(session.hostId, session.chatId)
    return true
  } catch (err) {
    report(err, { action: `Couldn't kill the agent on ${session.title}` })
    return false
  }
}

export async function confirmAndKillAgent(session: KillableAgent): Promise<boolean> {
  const { title, message } = killAgentConfirmation(session)
  const confirmed = await primitiveConfirm(message, {
    title,
    confirmLabel: 'Kill agent',
    cancelLabel: 'Keep running',
    destructive: true,
  })
  return confirmed && killAgent(session)
}
