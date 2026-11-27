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
  hostId: string
  chatId: string
  providerName: string
  presentation: ChatPresentation
  promptReplacing: boolean
}

function attachmentOf(
  chat: Pick<AgentChat, 'liveRunnerId' | 'phase' | 'terminalSessionId'> | null,
  providerName: string,
): TerminalAttachment {
  if (!chat) return { state: 'pending' }
  if (chat.liveRunnerId) return { state: 'attached', sessionId: chat.terminalSessionId || null }
  if (chat.phase === 'starting' || chat.phase === 'switching') {
    return { state: 'reviving', message: `Starting ${providerName}…` }
  }
  return { state: 'idle' }
}

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
  const connecting =
    chatSide && (phase === 'starting' || phase === 'switching')
      ? `Connecting to ${providerName}…`
      : undefined

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
    canSend: !offline && attachment.state !== 'pending',
    offline,
    startSession,
  }
}
