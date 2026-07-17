import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { AgentChat, AgentProvider } from '@/features/agent/api/agent-api'

const orderKey = (wsId: string) => `crowbar:agent-chat-order:${wsId}`

function loadOrder(wsId: string): string[] {
  try {
    const raw = localStorage.getItem(orderKey(wsId))
    return raw ? (JSON.parse(raw) as string[]) : []
  } catch {
    return []
  }
}

function saveOrder(wsId: string, order: string[]): void {
  try {
    localStorage.setItem(orderKey(wsId), JSON.stringify(order))
  } catch {
    /* quota / private mode — best effort */
  }
}

/** Order chats by the client-persisted order first (chats named in `order`, in
 *  that sequence), then append any chat absent from `order` sorted by createdAt
 *  ascending (creation order, newest last) — default ordering. Pure/testable. */
export function orderedChats(chats: AgentChat[], order: string[]): AgentChat[] {
  const byId = new Map(chats.map((c) => [c.id, c]))
  const pinned = order.map((id) => byId.get(id)).filter((c): c is AgentChat => c !== undefined)
  const pinnedIds = new Set(pinned.map((c) => c.id))
  const rest = chats
    .filter((c) => !pinnedIds.has(c.id))
    .sort((a, b) => a.createdAt.localeCompare(b.createdAt))
  return [...pinned, ...rest]
}

export interface AgentChatsState {
  chats: AgentChat[]
  /**
   * Is this chat's agent busy — the spinner map, keyed by chat id.
   *
   * NOT derived here. Every value is the server's own folded answer
   * (domain.AgentChat.Working), carried on the lifecycle frame that announced the
   * change and written through verbatim.
   *
   * That is deliberate and load-bearing. This map used to be re-derived from the
   * frame KIND — `turn_stopped` → idle — which stopped being true the moment a CLI
   * could hand work to a background subagent and go quiet waiting for it: claude
   * ends its turn right there, so the row went dark under an agent that was still
   * working. Re-deriving it here at all means a second copy of a rule that already
   * exists in Go, and two copies can disagree. One fold, on the server; this map
   * just displays it.
   */
  working: Record<string, boolean>
  order: string[]
  activeChatId: string | null
  providers: AgentProvider[]
}

export interface AgentChatsSlice {
  agentChats: AgentChatsState
  seedAgentChats: (chats: AgentChat[]) => void
  upsertAgentChat: (chat: AgentChat) => void
  removeAgentChat: (chatId: string) => void
  /** Write the server's folded busy state for a chat. Never computed client-side. */
  setAgentChatWorking: (chatId: string, working: boolean) => void
  setAgentChatOrder: (order: string[]) => void
  hydrateAgentChatOrder: () => void
  setActiveAgentChatId: (chatId: string | null) => void
  setAgentProviders: (providers: AgentProvider[]) => void
}

export const INITIAL_AGENT_CHATS_STATE: AgentChatsState = {
  chats: [],
  working: {},
  order: [],
  activeChatId: null,
  providers: [],
}

export const createAgentChatsSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  AgentChatsSlice
> = (set, get) => ({
  agentChats: { ...INITIAL_AGENT_CHATS_STATE },

  // Reconcile the chat list against an AUTHORITATIVE GET — the initial load and
  // every WS-reconnect reseed. Unlike a loop of upserts, this is a full
  // replacement, because the reseed's whole job is to repair what the socket
  // missed while it was down:
  //
  //  - Chats absent from the response are DROPPED. A `deleted` frame lost during
  //    the outage would otherwise leave a ghost row that never goes away.
  //  - The working map is CLEARED. Working state is not carried in the seed, so it
  //    is UNKNOWN at this point, and spec §2 mandates unknown → idle. Keeping it
  //    would strand a spinner forever on any row whose `turn_stopped` was dropped
  //    during the outage (until that chat happens to run another turn).
  seedAgentChats: (chats) =>
    set((s) => {
      const present = new Set(chats.map((c) => c.id))
      s.agentChats.chats = chats
      s.agentChats.working = {}
      s.agentChats.order = s.agentChats.order.filter((id) => present.has(id))
      if (s.agentChats.activeChatId !== null && !present.has(s.agentChats.activeChatId)) {
        s.agentChats.activeChatId = null
      }
    }),

  // Upsert ONE chat, refetched because a WS frame said it changed.
  //
  // A runner is placed on exactly ONE chat — the backend enforces it ("of everyone
  // here, the newest arrival stays and the rest go"), and this projection has to hold
  // the same invariant, because it is updated one chat at a time.
  //
  // The case that forces it: a runner MOVES (the user typed /clear inside the CLI).
  // The `moved` frame names the chat it moved INTO, so only that chat is refetched —
  // and the chat it LEFT keeps a liveRunnerId that is now a lie. Two chats would claim
  // one runner, and a pane following that runner would resolve to whichever came first
  // in the array (the stale one) and never follow. So an arriving chat evicts its
  // runner from wherever else it was: the fresh, server-sourced fact wins.
  //
  // This can only ever CLEAR a claim that a newer backend read contradicts — it never
  // invents liveness. terminalSessionId goes with it: a chat with no runner has no PTY
  // to attach, and leaving one behind would let a pane attach a dead session.
  upsertAgentChat: (chat) =>
    set((s) => {
      const idx = s.agentChats.chats.findIndex((c) => c.id === chat.id)
      if (idx === -1) s.agentChats.chats.push(chat)
      else s.agentChats.chats[idx] = chat

      if (!chat.liveRunnerId) return
      for (const c of s.agentChats.chats) {
        if (c.id !== chat.id && c.liveRunnerId === chat.liveRunnerId) {
          c.liveRunnerId = ''
          c.terminalSessionId = ''
        }
      }
    }),

  removeAgentChat: (chatId) =>
    set((s) => {
      s.agentChats.chats = s.agentChats.chats.filter((c) => c.id !== chatId)
      delete s.agentChats.working[chatId]
      s.agentChats.order = s.agentChats.order.filter((id) => id !== chatId)
      if (s.agentChats.activeChatId === chatId) s.agentChats.activeChatId = null
    }),

  setAgentChatWorking: (chatId, working) =>
    set((s) => {
      s.agentChats.working[chatId] = working
    }),

  setAgentChatOrder: (order) => {
    saveOrder(get().workspaceId, order)
    set((s) => {
      s.agentChats.order = order
    })
  },

  hydrateAgentChatOrder: () =>
    set((s) => {
      s.agentChats.order = loadOrder(get().workspaceId)
    }),

  setActiveAgentChatId: (chatId) =>
    set((s) => {
      s.agentChats.activeChatId = chatId
    }),

  setAgentProviders: (providers) =>
    set((s) => {
      s.agentChats.providers = providers
    }),
})
