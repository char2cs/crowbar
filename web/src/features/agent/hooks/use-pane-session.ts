import { useCallback } from 'react'
import { useStore } from 'zustand'
import { resumeChat } from '@/features/agent/api/agent-api'
import type { ComposerRevival } from '@/features/agent/composer/lib/composer-state'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { describeDormant, describeRung, sessionView } from '@/features/agent/lib/session-status'
import type { TerminalAttachment } from '@/features/agent/terminal/agent-terminal-surface'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

export interface PaneSessionInputs {
  store: WorkspaceStore
  wsId: string
  chatId: string
  providerName: string
  presentation: ChatPresentation
  /** A send's own replacement is in flight: the pane is mid-send, not waiting. */
  promptReplacing: boolean
}

export interface PaneSession {
  /** The chat list carries this chat. */
  known: boolean
  liveRunnerId: string
  attachment: TerminalAttachment
  /** The daemon placing a CLI, in the composer's words (chat side only). */
  revival?: ComposerRevival
  /** Why a dormant chat is dormant, or that a revive continued from the transcript. */
  sessionNote?: string
  /** A send may be dispatched: live, or dormant (the send revives it). */
  canSend: boolean
  /** The terminal surface's "Start session": resume without sending. */
  startSession: () => void
}

/**
 * The pane's view of its chat's session, DERIVED from the daemon's snapshot
 * (liveRunnerId, phase, session) and nothing else. The pane never orchestrates
 * a revive: a send revives a dormant chat server-side, and startSession is the
 * one explicit intent.
 */
export function usePaneSession({
  store,
  wsId,
  chatId,
  providerName,
  presentation,
  promptReplacing,
}: PaneSessionInputs): PaneSession {
  const known = useStore(store, (s) => s.agentChats.chats.some((c) => c.id === chatId))
  const liveRunnerId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.liveRunnerId ?? '',
  )
  // NOT a second liveness signal: a non-hotswap api-transport runner (codex) is
  // legitimately live with nothing attached.
  const sessionId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.terminalSessionId ?? '',
  )
  const phase = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.phase ?? 'dormant',
  )
  const exitReason = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.session?.exitReason ?? '',
  )
  const rung = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.session?.rung,
  )

  const attachment = deriveAttachment(
    sessionView({ known, liveRunnerId, phase, exitReason }),
    providerName,
    sessionId,
  )
  const chatSide = presentation !== 'terminal' && !promptReplacing
  const revival: ComposerRevival | undefined =
    chatSide && attachment.state === 'reviving'
      ? { state: 'reviving', message: attachment.message }
      : undefined
  const sessionNote = !chatSide
    ? undefined
    : attachment.state === 'idle'
      ? attachment.message
      : attachment.state === 'attached'
        ? describeRung({ rung })
        : undefined

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
    canSend: attachment.state === 'attached' || attachment.state === 'idle',
    startSession,
  }
}

function deriveAttachment(
  view: ReturnType<typeof sessionView>,
  providerName: string,
  sessionId: string,
): TerminalAttachment {
  switch (view.state) {
    case 'pending':
      return { state: 'pending' }
    case 'starting':
      return { state: 'reviving', message: `Starting ${providerName}…` }
    case 'dormant':
      return { state: 'idle', message: describeDormant(view.exitReason) }
    case 'live':
      // The attach-only view is bound to the PTY it names; a replacement PTY is a
      // swap of that same view, never a remount.
      return { state: 'attached', sessionId: sessionId || null }
  }
}
