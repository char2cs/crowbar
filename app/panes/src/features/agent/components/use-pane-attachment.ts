import { useCallback } from 'react'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import { resumeChat } from '@/features/agent/api/agent-chat-commands'
import type { ComposerRevival } from '@/features/agent/composer/lib/composer-state'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { readSelect, selectAgentChat } from '@/core/entities/select'
import { useHostDotState } from '@/features/hosts/hooks/use-host-status'
import { hostStatusOf } from '@/features/hosts/lib/host-status'
import { hostIsUnreachable } from '@/features/hosts/lib/host-tone'
import type { TerminalAttachment } from '@/features/agent/terminal/agent-terminal-surface'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

interface PaneAttachmentInputs {
  wsId: string
  /** The host the chat runs on; its reachability decides `offline`. */
  hostId: string
  chatId: string
  providerName: string
  presentation: ChatPresentation
  /** A send's own replacement is in flight: the pane is mid-send, not waiting. */
  promptReplacing: boolean
}

function attachmentOf(
  chat: Pick<AgentChat, 'liveRunnerId' | 'phase' | 'terminalSessionId'> | null,
  providerName: string,
): TerminalAttachment {
  if (!chat) return { state: 'pending' }
  // The attach-only view is bound to the PTY it names; a replacement is a swap.
  if (chat.liveRunnerId) return { state: 'attached', sessionId: chat.terminalSessionId || null }
  if (chat.phase === 'starting' || chat.phase === 'switching') {
    return { state: 'reviving', message: `Starting ${providerName}…` }
  }
  // A dormant chat says nothing: the next message starts its CLI.
  return { state: 'idle' }
}

/** The pane's view of its chat's session: liveness from the entity snapshot. The pane never orchestrates a revive:
 *  a send revives a dormant chat server-side, and startSession is the one intent. */
export function usePaneAttachment({
  wsId,
  hostId,
  chatId,
  providerName,
  presentation,
  promptReplacing,
}: PaneAttachmentInputs) {
  const offline = hostIsUnreachable(useHostDotState(hostId))
  const known = useChatField(chatId, (c) => c !== undefined)
  const liveRunnerId = useChatField(chatId, (c) => c?.liveRunnerId ?? '')
  // Not a second liveness signal: an api-transport runner is live with no PTY.
  const terminalSessionId = useChatField(chatId, (c) => c?.terminalSessionId ?? '')
  const phase = useChatField(chatId, (c) => c?.phase ?? 'dormant')

  const attachment = attachmentOf(
    known ? { liveRunnerId, phase, terminalSessionId } : null,
    providerName,
  )
  const chatSide = presentation !== 'terminal' && !promptReplacing
  const revival: ComposerRevival | undefined =
    chatSide && attachment.state === 'reviving'
      ? { state: 'reviving', message: attachment.message }
      : undefined
  // A CLI starting or moving onto the chat surface: sends queue, but nothing answers yet.
  const connecting =
    chatSide && (phase === 'starting' || phase === 'switching')
      ? `Connecting to ${providerName}…`
      : undefined

  // Reads the chat and its host as they are when called: every way onto the terminal may call
  // it, and only a known, dormant chat on a host that answers is started.
  const startSession = useCallback(() => {
    if (hostIsUnreachable(hostStatusOf(hostId))) return
    const chat = readSelect((s) => selectAgentChat(s, chatId))
    if (!chat || chat.liveRunnerId || chat.phase === 'starting' || chat.phase === 'switching')
      return
    void resumeChat(wsId, chatId).catch((err: unknown) => {
      toastSpawnFailure(err, providerName, 'resume')
    })
  }, [hostId, wsId, chatId, providerName])

  return {
    known,
    liveRunnerId,
    attachment,
    revival,
    connecting,
    // A prompt queues on any chat that exists, a spawning one included, while its host answers.
    canSend: !offline && attachment.state !== 'pending',
    /** The chat's host does not answer: the pane shows what it last held, read-only. */
    offline,
    startSession,
  }
}
