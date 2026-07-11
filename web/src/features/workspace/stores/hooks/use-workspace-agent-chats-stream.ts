import { useEffect } from 'react'
import { wsManager } from '@/lib/ws/manager'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { listChats, getChat, listProviders } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

// closeChatTab closes a deleted chat's pane tab the way the tab's own × button
// does: remove it from every pane holding it FIRST, then drop the buffer.
//
// Raw closeBuffer is NOT enough, and the shortcut cost a live bug: it only filters
// the buffer out of the buffers array, leaving the pane's activeBufferId pointing
// at a buffer that no longer exists. Deleting the chat you were looking at then
// blanked the whole pane — the remaining tab was still in the tab bar, but the
// pane rendered its empty "New Terminal" state until you clicked that tab.
// pane-slice's removeBufferFromPane is what activates an adjacent tab instead.
function closeChatTab(st: ReturnType<WorkspaceStore['getState']>, chatId: string): void {
  const buf = st.buffers.find((b) => b.type === 'agentChat' && b.chatId === chatId)
  if (!buf) return
  for (const pane of Object.values(st.panes ?? {})) {
    if (pane.bufferIds.includes(buf.id)) st.paneActions.removeBufferFromPane(pane.id, buf.id)
  }
  st.bufferActions.closeBuffer(buf.id)
}

// Bare lifecycle frame (00 agentic-engine spec §7): no snapshot, so most kinds
// react-then-refetch; only turn_started/turn_stopped carry enough in the kind
// itself to update the store without a round trip.
interface AgentChatEvent {
  chatId: string
  workspaceId: string
  kind:
    | 'created'
    | 'segment_opened'
    | 'segment_ended'
    | 'session_bound'
    | 'turn_started'
    | 'turn_stopped'
    | 'title_set'
    | 'deleted'
}

/**
 * Subscribe to the workspace-scoped agent-chat lifecycle WS while `wsId` is
 * active. Mirrors useWorkspaceThreadsStream: seed via GET, subscribe, handle the
 * {reconnected} sentinel by reseeding, and per-kind react-then-refetch vs
 * working-map toggle:
 *
 *  - turn_started / turn_stopped: the kind alone is enough — toggle the
 *    working map directly, no refetch.
 *  - created: a new chat (and its ordering) may have appeared — reseed the
 *    whole list.
 *  - title_set / segment_opened / segment_ended / session_bound: refetch just
 *    the affected chat and upsert (so activeSegmentId/activeProviderId/title
 *    update).
 *  - deleted: drop the chat from the store and close its pane tab if open.
 */
export function useWorkspaceAgentChatsStream(wsId: string): void {
  useEffect(() => {
    let cancelled = false

    // The seed is a full RECONCILE, not a merge: it runs on first load AND on every
    // reconnect, and on reconnect it is the only thing that can repair frames the
    // socket dropped while it was down. seedAgentChats therefore drops chats the
    // server no longer has (a missed `deleted`) and clears the working map (a missed
    // `turn_stopped` must not strand a spinner; working is unknown here → idle).
    const seedChats = async () => {
      try {
        const chats = await listChats(wsId)
        if (cancelled) return
        const store = getOrCreateWorkspaceStore(wsId)
        const before = store.getState()
        before.hydrateAgentChatOrder()

        const present = new Set(chats.map((c) => c.id))
        const vanished = before.agentChats.chats.filter((c) => !present.has(c.id)).map((c) => c.id)

        store.getState().seedAgentChats(chats)

        // A chat deleted during the outage never delivered its `deleted` frame, so
        // close its pane tab here exactly as that frame's handler would have.
        for (const chatId of vanished) closeChatTab(store.getState(), chatId)
      } catch {
        /* seed failure is non-fatal — the WS stream still pushes */
      }
    }

    const seedProviders = async () => {
      try {
        const providers = await listProviders(wsId)
        if (cancelled) return
        getOrCreateWorkspaceStore(wsId).getState().setAgentProviders(providers)
      } catch {
        /* non-fatal */
      }
    }

    const refetchOne = async (chatId: string) => {
      try {
        const chat = await getChat(wsId, chatId)
        if (cancelled) return
        getOrCreateWorkspaceStore(wsId).getState().upsertAgentChat(chat)
      } catch {
        /* a not-found here is handled by the deleted frame path */
      }
    }

    void seedChats()
    void seedProviders()

    const unsubscribe = wsManager.subscribe(`${workspaceBase(wsId)}/agent/ws/chats`, (frame) => {
      if (cancelled) return
      // Reconnect sentinel emitted by the manager after a socket drop+reopen —
      // reseed so pushes missed during the outage aren't lost.
      if (frame && typeof frame === 'object' && 'reconnected' in frame) {
        void seedChats()
        return
      }
      const ev = frame as AgentChatEvent
      if (!ev.chatId) return
      const st = getOrCreateWorkspaceStore(wsId).getState()
      switch (ev.kind) {
        case 'turn_started':
          st.setAgentChatWorking(ev.chatId, true)
          return
        case 'turn_stopped':
          st.setAgentChatWorking(ev.chatId, false)
          return
        case 'deleted': {
          st.removeAgentChat(ev.chatId)
          closeChatTab(st, ev.chatId)
          return
        }
        case 'created':
          void seedChats() // new chat + ordering — reseed the whole list
          return
        default:
          void refetchOne(ev.chatId) // title_set / segment_* / session_bound
      }
    })

    return () => {
      cancelled = true
      unsubscribe()
    }
  }, [wsId])
}
