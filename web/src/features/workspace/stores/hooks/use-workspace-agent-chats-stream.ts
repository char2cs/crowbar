import { useEffect } from 'react'
import { wsManager } from '@/lib/ws/manager'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { listChats, getChat, listProviders } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

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

    const seedChats = async () => {
      try {
        const chats = await listChats(wsId)
        if (cancelled) return
        const st = getOrCreateWorkspaceStore(wsId).getState()
        st.hydrateAgentChatOrder()
        for (const c of chats) st.upsertAgentChat(c)
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
          // Close the deleted chat's pane tab if open.
          const buf = st.buffers.find((b) => b.type === 'agentChat' && b.chatId === ev.chatId)
          if (buf) st.bufferActions.closeBuffer(buf.id)
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
