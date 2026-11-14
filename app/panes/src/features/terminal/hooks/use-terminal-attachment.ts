import { useEffect, useRef, useState } from 'react'
import {
  openTerminal,
  terminalCreate,
  terminalListLive,
  type TerminalConnection,
} from '@/lib/crowbar-bridge'
import { chatTerminalsBaseForWorkspace, terminalsBaseForWorkspace } from '@/lib/workspace-scope-url'
import { toast } from '@/features/window/stores/toast-store'
import { hostIdForWorkspace, isHostResolved, subscribeHostResolution } from '@/lib/host-resolve'
import { reportHostFailure } from '@/features/hosts/lib/host-failures'
import { toastHostFailure } from '@/features/hosts/lib/host-failure-notice'
import { onHostUp } from '@/features/hosts/lib/host-status'
import { resolveTerminalSession } from '../components/resolve-terminal-connection'
import { saveReconnect } from '../lib/terminal-reconnect-map'
import { useTerminalStore } from '../stores/terminal-store'

interface UseTerminalAttachmentOptions {
  /** The session this view shows (a terminal tab's id; an agent pane's PTY id). */
  sessionId: string
  /** The workspace this terminal belongs to — the one that places it on a host. */
  workspaceId: string
  /** The chat that owns the PTY, when the caller knows it (an agent pane). */
  chatId?: string
  /** Never spawn — see resolveTerminalSession. */
  attachOnly: boolean
  /** True once the view has been shown (a hidden tab costs nothing until looked at) and its
   *  terminals base can be built (the owning chat is recorded asynchronously). */
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

// One path for first attach, re-attach after a drop, and session swap; a generation stamp lets the
// newest attempt supersede older ones, so racing swaps converge without a lock. Each view has its
// own transport: an attach is a full redraw, and closing it never touches another view.
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

  // react-doctor-disable-next-line effect-needs-cleanup -- FP: the store subscriptions made inside attach() are held in stopWatching, which the returned cleanup calls (pinned by the "unmount while waiting" test).
  useEffect(() => {
    if (!enabled) return
    let generation = 0
    let current: TerminalConnection | null = null
    let stopWatching: (() => void) | null = null

    const attach = async () => {
      const mine = ++generation
      stopWatching?.()
      stopWatching = null
      const wsId = workspaceId
      // The project list names the host and a paired install has not read it at boot:
      // address the terminal nowhere until it has.
      if (!isHostResolved({ wsId })) {
        stopWatching = subscribeHostResolution(wsId, () => {
          if (isHostResolved({ wsId })) void attach()
        })
        return
      }
      let hostId: string
      let base: string
      try {
        hostId = hostIdForWorkspace(wsId)
        // Owner first, worktree second: an explicit chatId is this PTY's owner; a
        // shell tab falls back to the chat owning wsId's worktree.
        base = chatId
          ? chatTerminalsBaseForWorkspace(wsId, chatId)
          : terminalsBaseForWorkspace(wsId)
      } catch (err) {
        console.error('[terminal] cannot address the terminal:', err)
        return
      }
      const existing = useTerminalStore.getState().getSession(sessionId)
      let result
      try {
        result = await resolveTerminalSession({
          workspaceId: wsId,
          tabSessionId: sessionId,
          storeConnectionId: existing?.connectionId,
          // A host that cannot be reached says so now, not at its next probe: that probe's
          // 'up' is then the return this terminal waits for.
          listLiveSessions: () =>
            terminalListLive(base).catch((err: unknown) => {
              reportHostFailure(err)
              throw err
            }),
          createTerminal: () => terminalCreate(base, existing?.profileId),
          attachOnly,
        })
      } catch (err) {
        console.error('[terminal] attach failed:', err)
        if (!toastHostFailure(err)) {
          toast.error(
            'Terminal unavailable',
            'Could not start the terminal. Try reopening the tab.',
          )
        }
        return
      }
      // A PTY the daemon made is this tab's even if the host cannot be dialled now, or this
      // attempt was superseded and nothing newer has recorded one: remember it first, or the
      // next attach makes a second. A superseded answer never overwrites a newer attempt's id.
      const recorded = useTerminalStore.getState().getSession(sessionId)?.connectionId
      if ('sessionId' in result && (mine === generation || recorded === existing?.connectionId)) {
        useTerminalStore.getState().updateSession(sessionId, { connectionId: result.sessionId })
        saveReconnect(wsId, sessionId, result.sessionId)
      }
      if (mine !== generation) return // superseded while the daemon answered
      // The host was revoked while it answered: wait for it again rather than dial it.
      if (!isHostResolved({ wsId })) {
        void attach()
        return
      }
      // Could not ask the daemon (it is restarting, say): not a death. Stay detached
      // and ask again the moment THIS terminal's host is up again.
      if ('unknown' in result) {
        stopWatching = onHostUp(hostId, () => void attach())
        return
      }
      if ('gone' in result) {
        onEndedRef.current(sessionId)
        return
      }
      let connection: TerminalConnection
      try {
        connection = openTerminal(result.sessionId, base)
      } catch (err) {
        console.error('[terminal] attach failed:', err)
        toast.error('Terminal unavailable', 'Could not start the terminal. Try reopening the tab.')
        return
      }
      current = connection
      connection.onDrop(() => {
        if (current !== connection) return
        current = null
        setAttachment({ connection: null, created: false })
        void attach()
      })
      // A host that stops resolving (revoked, its project gone) is not streamed from.
      stopWatching = subscribeHostResolution(wsId, () => {
        if (current !== connection || isHostResolved({ wsId })) return
        current = null
        connection.close()
        setAttachment({ connection: null, created: false })
        void attach()
      })
      setAttachment({ connection, created: result.created })
    }

    void attach()
    return () => {
      generation++
      stopWatching?.()
      current?.close()
      current = null
      setAttachment({ connection: null, created: false })
    }
  }, [enabled, sessionId, workspaceId, chatId, attachOnly])

  return attachment
}
