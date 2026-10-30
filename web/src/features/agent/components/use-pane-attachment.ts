import { useCallback } from 'react'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import { resumeChat } from '@/features/agent/api/agent-chat-commands'
import type { ComposerRevival } from '@/features/agent/composer/lib/composer-state'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { TerminalAttachment } from '@/features/agent/terminal/agent-terminal-surface'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

interface PaneAttachmentInputs {
  wsId: string
  chatId: string
  providerName: string
  presentation: ChatPresentation
  /** A send's own replacement is in flight: the pane is mid-send, not waiting. */
  promptReplacing: boolean
}

/** Why a dormant chat has no CLI, in one sentence, and what continues it. */
function describeDormant(exitReason: string): string {
  switch (exitReason) {
    case 'stopped':
      return 'Stopped. Send a message to continue the conversation.'
    case 'daemon_restart':
      return 'The agent ended when Crowbar restarted. Send a message to continue.'
    case 'connection_lost':
    case 'transport_overflow':
      return 'Crowbar lost its connection to the agent. Send a message to continue.'
    case 'resume_failed':
      return "The provider could not reopen its session. Your next message continues from Crowbar's transcript."
    case 'moved':
      return 'The agent moved to another conversation. Send a message to continue this one.'
    case 'displaced':
      return 'Crowbar took the agent off this chat. Send a message to continue.'
    case 'spawn_failed':
      return 'The agent could not start. Check that its CLI is installed, then try again.'
    default:
      return 'The agent is not running. Send a message to continue the conversation.'
  }
}

function attachmentOf(
  chat: Pick<AgentChat, 'liveRunnerId' | 'phase' | 'terminalSessionId'> | null,
  exitReason: string,
  providerName: string,
): TerminalAttachment {
  if (!chat) return { state: 'pending' }
  // The attach-only view is bound to the PTY it names; a replacement is a swap.
  if (chat.liveRunnerId) return { state: 'attached', sessionId: chat.terminalSessionId || null }
  if (chat.phase === 'starting' || chat.phase === 'switching') {
    return { state: 'reviving', message: `Starting ${providerName}…` }
  }
  return { state: 'idle', message: describeDormant(exitReason) }
}

/**
 * The pane's view of its chat's session: liveness from the entity snapshot, the
 * exit from the last `runner_exited` row. The pane never orchestrates a revive:
 * a send revives a dormant chat server-side, and startSession is the one intent.
 */
export function usePaneAttachment({
  wsId,
  chatId,
  providerName,
  presentation,
  promptReplacing,
}: PaneAttachmentInputs) {
  const known = useChatField(chatId, (c) => c !== undefined)
  const liveRunnerId = useChatField(chatId, (c) => c?.liveRunnerId ?? '')
  // Not a second liveness signal: an api-transport runner is live with no PTY.
  const terminalSessionId = useChatField(chatId, (c) => c?.terminalSessionId ?? '')
  const phase = useChatField(chatId, (c) => c?.phase ?? 'dormant')
  const exitReason = useChatStore((s) => s.feeds[chatId]?.facts.lastExit?.reason ?? '')

  const attachment = attachmentOf(
    known ? { liveRunnerId, phase, terminalSessionId } : null,
    exitReason,
    providerName,
  )
  const chatSide = presentation !== 'terminal' && !promptReplacing
  const revival: ComposerRevival | undefined =
    chatSide && attachment.state === 'reviving'
      ? { state: 'reviving', message: attachment.message }
      : undefined
  const sessionNote = chatSide && attachment.state === 'idle' ? attachment.message : undefined

  const startSession = useCallback(() => {
    void resumeChat(wsId, chatId).catch((err: unknown) => {
      toastSpawnFailure(err, providerName, 'resume')
    })
  }, [wsId, chatId, providerName])

  return {
    known,
    liveRunnerId,
    attachment,
    revival,
    sessionNote,
    // A prompt queues on any chat that exists, a spawning one included.
    canSend: attachment.state !== 'pending',
    startSession,
  }
}
