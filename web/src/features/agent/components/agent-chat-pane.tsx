import { useCallback, useEffect, useState } from 'react'
import { useStore } from 'zustand'
import { Frame, FrameFooter, FramePanel } from '@/components/ui/frame'
import { getChat, switchProvider } from '@/features/agent/api/agent-api'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { toast } from '@/features/window/stores/toast-store'
import { terminalListLive } from '@/lib/crowbar-bridge'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { ProviderSwitchDropdown } from './provider-switch-dropdown'

// attachAgentSegment resolves chatId's ACTIVE segment's live terminal session and
// pre-seeds the terminal-store mapping (connectionId = terminalSessionId) plus the
// localStorage reconnect backstop, so XtermTerminal's resolveTerminalConnection
// ATTACHES the agent's already-running PTY instead of spawning a fresh shell.
//
// Why this attaches (and does not spawn): XtermTerminal mounts with
// `sessionId = terminalSessionId`, reads getSession(sessionId).connectionId — now
// pre-seeded to the same id — and passes it to resolveTerminalConnection as
// `storeConnectionId`. On a fresh mount there is no live WS transport yet, so the
// resolver lists the daemon's live sessions, finds this PTY in them, and calls
// terminalAttach (the in-memory-store reuse branch).
//
// It returns null — do NOT mount a terminal — whenever the agent's PTY is not
// provably alive. That guard is load-bearing, because resolveTerminalConnection's
// fallback for an unknown connection id is createTerminal(): seeding a dead id
// would make it spawn a fresh BARE SHELL inside the agent pane and persist that
// shell into the reconnect map. The window is real — a CLI can exit before its
// EndSegment command lands, leaving the chat's active segment briefly pointing at
// a PTY the daemon has already reaped — so both facts are checked:
//
//  1. the active segment is still `active` (an `ended` segment's CLI has exited), and
//  2. the daemon still lists its terminal session as live.
//
// A failed liveness listing is treated as not-live for the same reason: never
// spawn. The caller renders an "agent session ended" state instead, and the WS
// stream's next segment_* frame re-runs this attach.
export async function attachAgentSegment(wsId: string, chatId: string): Promise<string | null> {
  const chat = await getChat(wsId, chatId)
  const segment = chat.segments.find((s) => s.id === chat.activeSegmentId)
  if (!segment || segment.status !== 'active') return null

  const sessionId = segment.terminalSessionId
  if (!sessionId) return null

  const live = await terminalListLive(`${workspaceBase(wsId)}/terminals`).catch(
    () => [] as string[],
  )
  if (!live.includes(sessionId)) return null

  useTerminalStore.getState().updateSession(sessionId, { connectionId: sessionId })
  saveReconnect(wsId, sessionId, sessionId)
  return sessionId
}

// The pane's attach outcome. `pending` is the pre-resolution state and renders
// nothing — distinguishing it from `ended` keeps the empty state from flashing on
// every open while the segment is being resolved.
type Attachment =
  | { state: 'pending' }
  | { state: 'attached'; sessionId: string }
  | { state: 'ended' }

interface AgentChatPaneProps {
  chatId: string
  wsId: string
  /** The pane buffer backing this chat — the tab whose label tracks chat.title. */
  bufferId: string
  isActivePane: boolean
}

// Headerless CossUI Frame: a flush FramePanel holding the live agent terminal,
// and a FrameFooter whose only control is the provider-switch dropdown. The pane
// tab is keyed by the stable chatId; the inner terminal is keyed by the segment's
// terminalSessionId so a provider switch (new segment) remounts and re-attaches
// in place without disturbing the tab.
export function AgentChatPane({ chatId, wsId, bufferId, isActivePane }: AgentChatPaneProps) {
  const store = useWorkspaceStore()
  const activeSegmentId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.activeSegmentId ?? null,
  )
  const activeProviderId = useStore(
    store,
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.activeProviderId ?? '',
  )
  const title = useStore(store, (s) => s.agentChats.chats.find((c) => c.id === chatId)?.title ?? '')
  const providers = useStore(store, (s) => s.agentChats.providers)

  const [attachment, setAttachment] = useState<Attachment>({ state: 'pending' })

  // The tab label is a snapshot taken by openContent, but a chat's title changes
  // AFTER the tab opens: the agent auto-titles it (WS `title_set`) and the user
  // can rename it from the sidebar. Both land on this chat's `title` in the store,
  // so mirroring title → buffer name here keeps the open tab correct for both
  // paths, with no store → component dependency.
  useEffect(() => {
    if (!title) return
    const s = store.getState()
    const buffer = s.bufferActions.getBufferById(bufferId)
    if (buffer && buffer.name !== title) s.bufferActions.renameBuffer(bufferId, title)
  }, [store, bufferId, title])

  // (Re-)attach when the chat opens or its active segment changes. Keying the
  // terminal by the resolved sessionId below remounts it on a switch so the new
  // PTY is attached in place.
  useEffect(() => {
    let cancelled = false
    setAttachment({ state: 'pending' })
    void attachAgentSegment(wsId, chatId).then((sid) => {
      if (cancelled) return
      setAttachment(sid === null ? { state: 'ended' } : { state: 'attached', sessionId: sid })
    })
    return () => {
      cancelled = true
    }
  }, [wsId, chatId, activeSegmentId])

  // The mount guard (attachAgentSegment) only proves the PTY was alive at OPEN.
  // The agent's CLI can die at any moment while the pane sits here — daemon
  // restart, CLI exit, crash — and the terminal's transport-drop reconnect would,
  // by default, resolve the now-dead session by spawning a fresh BARE SHELL into
  // this frame. So the terminal runs attach-only and reports the session gone
  // instead; we render the same ended state the mount guard does. The backend's
  // boot reconcile independently ends the segment and the WS `segment_ended` frame
  // re-runs the effect above — but this guard must stand on its own, because the
  // PTY's death and the daemon's knowledge of it are not the same instant.
  const handleSessionGone = useCallback(() => setAttachment({ state: 'ended' }), [])

  // A provider switch is the headline interaction of this pane, and it can fail
  // for real, ordinary reasons — the target CLI is not installed, the spawn fails.
  // Without this catch the rejection is unhandled: the dropdown just closes and
  // nothing happens. Surface it, matching the write-path error handling in
  // agent-chats-panel (create/rename/delete).
  const handleSwitch = (providerId: string) => {
    switchProvider(wsId, chatId, providerId).catch((err: unknown) => {
      console.error('Failed to switch agent provider:', err)
      const name = providers.find((p) => p.id === providerId)?.displayName ?? providerId
      toast.error(
        'Could not switch provider',
        `Crowbar could not start ${name}. Check that its CLI is installed and on your PATH.`,
      )
    })
  }

  return (
    <Frame className="h-full w-full rounded-none bg-transparent p-0">
      <FramePanel className="min-h-0 flex-1 rounded-none border-0 bg-transparent p-0 shadow-none before:hidden">
        {attachment.state === 'attached' && (
          <XtermTerminal
            key={attachment.sessionId}
            sessionId={attachment.sessionId}
            isActive={isActivePane}
            isVisible
            attachOnly
            onSessionGone={handleSessionGone}
          />
        )}
        {attachment.state === 'ended' && (
          <div className="flex h-full w-full items-center justify-center p-6">
            <p className="text-muted-foreground text-center text-sm">
              This agent session has ended. Switch provider below to start a new one.
            </p>
          </div>
        )}
      </FramePanel>
      <FrameFooter className="flex items-center justify-start px-2 py-1.5">
        <ProviderSwitchDropdown
          providers={providers}
          currentProviderId={activeProviderId}
          onSwitch={handleSwitch}
        />
      </FrameFooter>
    </Frame>
  )
}
