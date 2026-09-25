import { useEffect, useRef, useState } from 'react'
import {
  openTerminal,
  terminalCreate,
  terminalListLive,
  type TerminalConnection,
} from '@/lib/crowbar-bridge'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { chatBase, terminalsBaseForWorkspace } from '@/lib/workspace-scope-url'
import { toast } from '@/features/window/stores/toast-store'
import { useConnectionStore } from '@/lib/ws/connection-store'
import { resolveTerminalSession } from '../components/resolve-terminal-connection'
import { saveReconnect } from '../lib/terminal-reconnect-map'
import { useTerminalStore } from '../stores/terminal-store'

interface UseTerminalAttachmentOptions {
  /** The session this view shows (a terminal tab's id; an agent pane's PTY id). */
  sessionId: string
  /** The workspace this terminal belongs to; falls back to the active one. */
  workspaceId?: string
  /** The chat that owns the PTY, when the caller knows it (an agent pane). */
  chatId?: string
  /** Never spawn — see resolveTerminalSession. */
  attachOnly: boolean
  /**
   * False until the view may attach at all: it has been shown at least once
   * (a hidden tab costs nothing until it is looked at), and its terminals base
   * can be built (a workspace's owning chat is recorded asynchronously).
   */
  enabled: boolean
  /** The session ended: its exit frame arrived, or the daemon no longer has it. */
  onEnded: (sessionId: string) => void
}

export interface TerminalAttachment {
  /** This view's live stream to its PTY, or null while (re)attaching. */
  connection: TerminalConnection | null
  /** True when the PTY was spawned by this attach (a tab's first shell). */
  created: boolean
}

/**
 * Owns this view's connection to its daemon PTY: ONE path for the first attach,
 * the re-attach after a transport drop, and the swap when the view is pointed at
 * a different session. Every attempt is stamped with a generation; a newer
 * attempt (a swap, a drop, an unmount) supersedes an older one still waiting on
 * the daemon, so racing swaps converge on the latest session with no lock, no
 * swallowed call and no reconcile pass.
 *
 * Each view has its own transport, and the daemon paints every attached client
 * with its own snapshot — so an attach is always a redraw, and closing this
 * view's transport (on unmount or swap) never touches another view's.
 */
export function useTerminalAttachment({
  sessionId,
  workspaceId,
  chatId,
  attachOnly,
  enabled,
  onEnded,
}: UseTerminalAttachmentOptions): TerminalAttachment {
  const [attachment, setAttachment] = useState<TerminalAttachment>({
    connection: null,
    created: false,
  })
  const onEndedRef = useRef(onEnded)
  useEffect(() => {
    onEndedRef.current = onEnded
  }, [onEnded])

  // react-doctor-disable-next-line effect-needs-cleanup -- FP: the connection-store subscription made inside attach() is held in stopWaiting, which the returned cleanup calls (pinned by the "unmount while waiting" test).
  useEffect(() => {
    if (!enabled) return
    let generation = 0
    let current: TerminalConnection | null = null
    let stopWaiting: (() => void) | null = null

    const attach = async () => {
      const mine = ++generation
      stopWaiting?.()
      stopWaiting = null
      const wsId = workspaceId ?? getActiveWorkspaceId()
      if (!wsId) return
      // Owner first, worktree second: an explicit chatId is this PTY's owner; a
      // shell tab falls back to the chat owning wsId's worktree.
      const base = chatId ? `${chatBase(chatId)}/terminals` : terminalsBaseForWorkspace(wsId)
      const existing = useTerminalStore.getState().getSession(sessionId)
      let result
      try {
        result = await resolveTerminalSession({
          workspaceId: wsId,
          tabSessionId: sessionId,
          storeConnectionId: existing?.connectionId,
          listLiveSessions: () => terminalListLive(base),
          createTerminal: () => terminalCreate(base, existing?.profileId),
          attachOnly,
        })
      } catch (err) {
        console.error('[terminal] attach failed:', err)
        toast.error('Terminal unavailable', 'Could not start the terminal. Try reopening the tab.')
        return
      }
      if (mine !== generation) return // superseded while the daemon answered
      // Could not ask the daemon (it is restarting, say): not a death. Stay detached
      // and ask again the moment the app's own streams report the daemon back.
      if ('unknown' in result) {
        stopWaiting = useConnectionStore.subscribe((state, prev) => {
          if (state.status === 'connected' && prev.status !== 'connected') void attach()
        })
        return
      }
      if ('gone' in result) {
        onEndedRef.current(sessionId)
        return
      }
      const connection = openTerminal(result.sessionId, base)
      current = connection
      connection.onDrop(() => {
        if (current !== connection) return
        current = null
        setAttachment({ connection: null, created: false })
        void attach()
      })
      useTerminalStore.getState().updateSession(sessionId, { connectionId: result.sessionId })
      saveReconnect(wsId, sessionId, result.sessionId)
      setAttachment({ connection, created: result.created })
    }

    void attach()
    return () => {
      generation++
      stopWaiting?.()
      current?.close()
      current = null
      setAttachment({ connection: null, created: false })
    }
  }, [enabled, sessionId, workspaceId, chatId, attachOnly])

  return attachment
}
