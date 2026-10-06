import { createStore } from 'zustand'
import { listChatMessages, type AgentChatMessage } from '@/features/agent/api/agent-api'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

const MESSAGE_PAGE_SIZE = 100
const EMPTY_MESSAGES: AgentChatMessage[] = []

export interface ChatLedger {
  messages: AgentChatMessage[]
  /** Highest sequence held; the forward refresh asks after it. */
  cursor: number
  /** Lowest sequence held; paging older asks before it. */
  oldestCursor: number
  hasOlder: boolean
  status: 'loading' | 'loaded' | 'error'
  error: Error | null
  /** Bumped per initial load so a superseded response is dropped. */
  generation: number
  /** The first-page request in flight, joined by every later caller. */
  pending: Promise<void> | null
}

interface ChatLedgerState {
  ledgers: Record<string, ChatLedger>
  /** Fetch the newest page once. Joins a load in flight and is a no-op once
   *  loaded; `force` supersedes whatever is there. Never rejects: a failure is
   *  recorded on the ledger. */
  loadInitial: (wsId: string, chatId: string, force?: boolean) => Promise<void>
  /** Fold fetched messages into the ledger; `older` also moves the backward
   *  paging extent. */
  merge: (
    chatId: string,
    incoming: AgentChatMessage[],
    older?: { oldestCursor: number; hasMore: boolean },
  ) => AgentChatMessage[]
  setError: (chatId: string, error: Error | null) => void
  evict: (chatId: string) => void
}

// Sorted by displayOrder (dispatch order), NOT sequence (persist order) — an
// interrupted turn that finishes late must still display before a later turn
// that finished first. The merge key is turnId, not sequence: a turn can be
// re-closed under a fresh sequence for the same turnId, and that is an update
// to the row, not a second row.
function mergeMessages(current: AgentChatMessage[], incoming: AgentChatMessage[]) {
  const byTurnId = new Map(current.map((item) => [item.turnId, item]))
  for (const item of incoming) byTurnId.set(item.turnId, item)
  return [...byTurnId.values()].sort(
    (a, b) =>
      (a.displayOrder ?? a.sequence) - (b.displayOrder ?? b.sequence) ||
      (a.itemIndex ?? 0) - (b.itemIndex ?? 0),
  )
}

function withMerged(ledger: ChatLedger, incoming: AgentChatMessage[]): ChatLedger {
  // An empty page keeps the array reference so an unchanged poll does not
  // re-render the transcript.
  if (incoming.length === 0) return ledger
  const messages = mergeMessages(ledger.messages, incoming)
  const oldest = messages[0]?.sequence ?? 0
  return {
    ...ledger,
    messages,
    cursor: Math.max(ledger.cursor, messages.at(-1)?.sequence ?? 0),
    oldestCursor: ledger.oldestCursor === 0 ? oldest : Math.min(ledger.oldestCursor, oldest),
  }
}

const blankLedger: ChatLedger = {
  messages: EMPTY_MESSAGES,
  cursor: 0,
  oldestCursor: 0,
  hasOlder: false,
  status: 'loading',
  error: null,
  generation: 0,
  pending: null,
}

export const chatLedgerStore = createStore<ChatLedgerState>()((set, get) => {
  const patch = (chatId: string, change: Partial<ChatLedger>) =>
    set((state) => {
      const ledger = state.ledgers[chatId]
      return ledger ? { ledgers: { ...state.ledgers, [chatId]: { ...ledger, ...change } } } : state
    })

  const fetchFirstPage = async (wsId: string, chatId: string, generation: number) => {
    const isCurrent = () => get().ledgers[chatId]?.generation === generation
    try {
      const page = await listChatMessages(wsId, chatId, { limit: MESSAGE_PAGE_SIZE })
      if (!isCurrent()) return
      const reset: ChatLedger = {
        ...get().ledgers[chatId],
        messages: EMPTY_MESSAGES,
        cursor: page.cursor,
        oldestCursor: page.oldestCursor,
        hasOlder: page.hasMore,
      }
      patch(chatId, {
        ...withMerged(reset, page.items),
        status: 'loaded',
        error: null,
        pending: null,
      })
    } catch (err) {
      if (!isCurrent()) return
      patch(chatId, {
        status: 'error',
        error: err instanceof Error ? err : new Error(String(err)),
        pending: null,
      })
    }
  }

  return {
    ledgers: {},
    loadInitial: (wsId, chatId, force = false) => {
      const current = get().ledgers[chatId]
      if (!force && current?.status === 'loaded') return Promise.resolve()
      if (!force && current?.pending) return current.pending
      const generation = (current?.generation ?? 0) + 1
      set((state) => ({
        ledgers: {
          ...state.ledgers,
          [chatId]: { ...(current ?? blankLedger), status: 'loading', error: null, generation },
        },
      }))
      const pending = fetchFirstPage(wsId, chatId, generation)
      patch(chatId, { pending })
      return pending
    },
    merge: (chatId, incoming, older) => {
      const ledger = get().ledgers[chatId]
      if (!ledger) return EMPTY_MESSAGES
      let next = withMerged(ledger, incoming)
      if (older) next = { ...next, oldestCursor: older.oldestCursor, hasOlder: older.hasMore }
      if (next !== ledger) patch(chatId, next)
      return next.messages
    },
    setError: (chatId, error) => {
      if (get().ledgers[chatId]?.error !== error) patch(chatId, { error })
    },
    evict: (chatId) =>
      set((state) => {
        if (!(chatId in state.ledgers)) return state
        const { [chatId]: _dropped, ...rest } = state.ledgers
        return { ledgers: rest }
      }),
  }
})

// A ledger lives exactly as long as some pane holds its chat: closing a view or
// deleting a chat drops the pane, which drops the ledger and its pages.
windowPaneStore.subscribe((state, prev) => {
  if (state.panes === prev.panes) return
  const held = new Set<string>()
  for (const pane of Object.values(state.panes)) if (pane.chatId) held.add(pane.chatId)
  const { ledgers, evict } = chatLedgerStore.getState()
  for (const chatId of Object.keys(ledgers)) if (!held.has(chatId)) evict(chatId)
})
