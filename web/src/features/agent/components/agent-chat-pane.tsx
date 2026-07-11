import { useEffect, useState } from 'react'
import { useStore } from 'zustand'
import { Frame, FrameFooter, FramePanel } from '@/components/ui/frame'
import { getChat, switchProvider } from '@/features/agent/api/agent-api'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
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
// terminalAttach (the in-memory-store reuse branch). Returns the session id to
// mount, or null when the active segment carries no live terminal session.
export async function attachAgentSegment(wsId: string, chatId: string): Promise<string | null> {
  const chat = await getChat(wsId, chatId)
  const segment = chat.segments.find((s) => s.id === chat.activeSegmentId)
  const sessionId = segment?.terminalSessionId
  if (!sessionId) return null
  useTerminalStore.getState().updateSession(sessionId, { connectionId: sessionId })
  saveReconnect(wsId, sessionId, sessionId)
  return sessionId
}

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

  const [sessionId, setSessionId] = useState<string | null>(null)

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
    void attachAgentSegment(wsId, chatId).then((sid) => {
      if (!cancelled) setSessionId(sid)
    })
    return () => {
      cancelled = true
    }
  }, [wsId, chatId, activeSegmentId])

  return (
    <Frame className="h-full w-full rounded-none bg-transparent p-0">
      <FramePanel className="min-h-0 flex-1 rounded-none border-0 bg-transparent p-0 shadow-none before:hidden">
        {sessionId && (
          <XtermTerminal key={sessionId} sessionId={sessionId} isActive={isActivePane} isVisible />
        )}
      </FramePanel>
      <FrameFooter className="flex items-center justify-start px-2 py-1.5">
        <ProviderSwitchDropdown
          providers={providers}
          currentProviderId={activeProviderId}
          onSwitch={(providerId) => void switchProvider(wsId, chatId, providerId)}
        />
      </FrameFooter>
    </Frame>
  )
}
