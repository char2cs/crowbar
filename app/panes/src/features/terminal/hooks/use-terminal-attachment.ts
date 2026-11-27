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
  sessionId: string
  workspaceId: string
  chatId?: string
  attachOnly: boolean
  enabled: boolean
  onEnded: (sessionId: string) => void
}

export interface TerminalAttachment {
  connection: TerminalConnection | null
  created: boolean
}

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
      const recorded = useTerminalStore.getState().getSession(sessionId)?.connectionId
      if ('sessionId' in result && (mine === generation || recorded === existing?.connectionId)) {
        useTerminalStore.getState().updateSession(sessionId, { connectionId: result.sessionId })
        saveReconnect(wsId, sessionId, result.sessionId)
      }
      if (mine !== generation) return
      if (!isHostResolved({ wsId })) {
        void attach()
        return
      }
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
