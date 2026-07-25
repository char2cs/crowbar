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
 *  DESCENDING — newest first. Pure/testable.
 *
 *  Newest-first is the default because a chat you just started is the one you
 *  want, and because the New Tab's "Recent" list takes its top-N straight from
 *  this function (new-tab-view.tsx): appending newest LAST buried the newest chat
 *  and, once past the list cap, hid it entirely. Chats the user has explicitly
 *  dragged still lead, in the sequence they chose — an explicit arrangement
 *  outranks recency. */
export function orderedChats(chats: AgentChat[], order: string[]): AgentChat[] {
  const byId = new Map(chats.map((c) => [c.id, c]))
  const pinned = order.map((id) => byId.get(id)).filter((c): c is AgentChat => c !== undefined)
  const pinnedIds = new Set(pinned.map((c) => c.id))
  const rest = chats
    .filter((c) => !pinnedIds.has(c.id))
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
  return [...pinned, ...rest]
}

/** The enabled providers, in the backend's priority order — the single subset
 *  every "New chat" surface offers (the first is the provider a unified New-chat
 *  action opens). A disabled provider is hidden entirely (spec §2.2), so it never
 *  appears here. Pure selector; use with a narrow `useStore(store, …)` read. */
export const selectEnabledProviders = (s: WorkspaceState): AgentProvider[] =>
  s.agentChats.providers.filter((p) => p.enabled)

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
  seedAgentChats: (chats: AgentChat[], opts?: { keepWorking?: boolean }) => void
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
  //
  // `keepWorking` is the one exception, and it exists for the `created` reseed. That
  // reseed rides a LIVE socket — it fires because a new chat appeared, not because the
  // connection dropped — so NO turn frame was missed and every surviving chat's working
  // state is still the truth. Clearing it there is a bug: it blanks the spinner on every
  // OTHER mid-turn chat the instant a new chat opens, until each runs another turn. So
  // when told working is known, keep it, and only forget entries for chats that are gone.
  seedAgentChats: (chats, opts) => {
    const prev = get().agentChats
    const present = new Set(chats.map((c) => c.id))
    const pruned = prev.order.filter((id) => present.has(id))

    // Chats that appeared since the last seed. A `created` frame reseeds the whole
    // list, so this is where a brand-new chat first shows up. Only meaningful once
    // a previous list EXISTS: on the first seed of a session every chat looks new,
    // and promoting them all would scramble the very arrangement being restored.
    // Both membership tests are Sets — this runs on every reseed, over the whole
    // list, so a nested scan here would be quadratic in the chat count.
    const knownIds = new Set(prev.chats.map((c) => c.id))
    const prunedIds = new Set(pruned)
    const arrived =
      prev.chats.length === 0
        ? []
        : chats
            .filter((c) => !knownIds.has(c.id) && !prunedIds.has(c.id))
            .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
            .map((c) => c.id)

    // A new chat joins the TOP of an existing arrangement. One drag pins the whole
    // list, so without this every later chat sinks below every pinned one — and off
    // the New Tab's capped "Recent" list entirely. With NO saved order the
    // newest-first sort in orderedChats already puts it first, and writing one here
    // would start pinning a list the user never arranged.
    const nextOrder = pruned.length > 0 && arrived.length > 0 ? [...arrived, ...pruned] : pruned

    set((s) => {
      s.agentChats.chats = chats
      if (opts?.keepWorking) {
        for (const id of Object.keys(s.agentChats.working)) {
          if (!present.has(id)) delete s.agentChats.working[id]
        }
      } else {
        s.agentChats.working = {}
      }
      s.agentChats.order = nextOrder
      if (s.agentChats.activeChatId !== null && !present.has(s.agentChats.activeChatId)) {
        s.agentChats.activeChatId = null
      }
    })

    if (arrived.length > 0 && nextOrder !== pruned) saveOrder(get().workspaceId, nextOrder)
  },

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
