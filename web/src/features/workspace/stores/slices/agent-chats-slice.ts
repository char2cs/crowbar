import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { clearPersistedPromptQueue } from '@/features/agent/lib/prompt-queue-persistence'
import {
  applySnapshot,
  reduceChatFrame,
  type ChatFrameOutcome,
} from '@/features/agent/lib/reduce-chat-frame'
import { clearScrollPosition } from '@/features/agent/hooks/lib/transcript-scroll-positions'
import {
  type AgentChatsState,
  type AgentChatsSlice,
  INITIAL_AGENT_CHATS_STATE,
} from './agent-chats-state'

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

/**
 * Apply one snapshot to the draft under the version rule, and mirror what it
 * carries into the per-chat maps consumers select narrowly. The ONE writer of
 * a chat's row, working flag and terminal wait.
 */
function applyOne(state: AgentChatsState, chat: AgentChat): ChatFrameOutcome {
  const { chats, outcome } = applySnapshot(state.chats, chat)
  if (outcome.kind !== 'applied') return outcome
  state.chats = chats as AgentChat[]
  mirror(state, outcome)
  return outcome
}

function mirror(
  state: AgentChatsState,
  outcome: Extract<ChatFrameOutcome, { kind: 'applied' }>,
): void {
  const { chat } = outcome
  if (chat.working) state.working[chat.id] = true
  else delete state.working[chat.id]
  if (chat.terminalWait) state.terminalWaits[chat.id] = chat.terminalWait
  else delete state.terminalWaits[chat.id]
}

export const createAgentChatsSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  AgentChatsSlice
> = (set, get) => ({
  agentChats: { ...INITIAL_AGENT_CHATS_STATE },

  seedAgentChats: (chats) => {
    const prev = get().agentChats
    const present = new Set(chats.map((c) => c.id))
    const vanished: string[] = []
    for (const chat of prev.chats) if (!present.has(chat.id)) vanished.push(chat.id)

    // Chats that appeared since the last seed join the TOP of an existing
    // arrangement. One drag pins the whole list, so without this every later
    // chat sinks below every pinned one — and off the New Tab's capped
    // "Recent" list entirely. With NO saved order the newest-first sort in
    // orderedChats already puts it first, and writing one here would start
    // pinning a list the user never arranged. Only meaningful once a list
    // exists: on the first seed every chat looks new.
    const known = new Set(prev.chats.map((c) => c.id))
    const arrived =
      prev.chats.length === 0 || prev.order.length === 0
        ? []
        : chats
            .filter((c) => !known.has(c.id))
            .sort((a, b) => b.createdAt.localeCompare(a.createdAt))
            .map((c) => c.id)

    set((s) => {
      for (const chat of chats) applyOne(s.agentChats, chat)
      s.agentChats.listSeeded = true
      if (arrived.length > 0) s.agentChats.order = [...arrived, ...s.agentChats.order]
    })
    if (arrived.length > 0) saveOrder(get().workspaceId, get().agentChats.order)
    return vanished
  },

  // Both appliers reduce the PLAIN (frozen) state outside the Immer draft, so
  // the outcome handed back — previous row included — is never a revoked proxy.
  applyAgentChat: (chat) => {
    const { chats, outcome } = applySnapshot(get().agentChats.chats, chat)
    if (outcome.kind === 'applied') {
      set((s) => {
        s.agentChats.chats = chats as AgentChat[]
        mirror(s.agentChats, outcome)
      })
    }
    return outcome
  },

  applyAgentChatFrame: (frame) => {
    const { chats, outcome } = reduceChatFrame(get().agentChats.chats, frame)
    if (outcome.kind === 'applied') {
      // A chat the store has never held joins the TOP of an existing
      // arrangement — the same rule seedAgentChats applies to a list read.
      const arrived = outcome.previous === undefined && get().agentChats.order.length > 0
      set((s) => {
        s.agentChats.chats = chats as AgentChat[]
        mirror(s.agentChats, outcome)
        if (arrived && !s.agentChats.order.includes(outcome.chat.id)) {
          s.agentChats.order = [outcome.chat.id, ...s.agentChats.order]
        }
      })
      if (arrived) saveOrder(get().workspaceId, get().agentChats.order)
    }
    if (outcome.kind === 'deleted') get().removeAgentChat(outcome.chatId)
    return outcome
  },

  removeAgentChat: (chatId) => {
    set((s) => {
      s.agentChats.chats = s.agentChats.chats.filter((c) => c.id !== chatId)
      delete s.agentChats.working[chatId]
      delete s.agentChats.terminalWaits[chatId]
      delete s.agentChats.telemetry[chatId]
      delete s.agentChats.excalidrawEditRequests[chatId]
      s.agentChats.order = s.agentChats.order.filter((id) => id !== chatId)
      if (s.agentChats.activeChatId === chatId) s.agentChats.activeChatId = null
    })
    clearPersistedPromptQueue(get().workspaceId, chatId)
    // Not part of this store — see transcript-scroll-positions.ts's own doc.
    clearScrollPosition(chatId)
  },

  setAgentChatTelemetry: (chatId, report) =>
    set((s) => {
      if (report) s.agentChats.telemetry[chatId] = report
      else delete s.agentChats.telemetry[chatId]
    }),

  requestExcalidrawEdit: (chatId, scene) =>
    set((s) => {
      s.agentChats.excalidrawEditRequests[chatId] = scene
    }),

  clearExcalidrawEditRequest: (chatId) =>
    set((s) => {
      delete s.agentChats.excalidrawEditRequests[chatId]
    }),

  setAgentChatSelection: (chatId, model, effort) =>
    set((s) => {
      const chat = s.agentChats.chats.find((c) => c.id === chatId)
      if (!chat) return
      // Both halves, always — they are one answer. The endpoint takes the whole
      // selection because which effort levels are valid is a property of the
      // model, and writing one half here would let the store hold a pair that was
      // never jointly sent.
      chat.model = model
      chat.effort = effort
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

  seedAgentChatFolders: (folders) =>
    set((s) => {
      s.agentChats.folders = folders
    }),

  applyAgentChatFolders: (folders) =>
    set((s) => {
      const at = new Map(s.agentChats.folders.map((f, i) => [f.id, i]))
      for (const folder of folders) {
        const i = at.get(folder.id)
        if (i === undefined) {
          at.set(folder.id, s.agentChats.folders.length)
          s.agentChats.folders.push(folder)
        } else {
          s.agentChats.folders[i] = folder
        }
      }
    }),

  removeAgentChatFolder: (folderId) =>
    set((s) => {
      const gone = s.agentChats.folders.find((f) => f.id === folderId)
      if (!gone) return
      const grandparent = gone.parentId ?? ''
      s.agentChats.folders = s.agentChats.folders.filter((f) => f.id !== folderId)
      for (const f of s.agentChats.folders) {
        if ((f.parentId ?? '') === folderId) f.parentId = grandparent
      }
      for (const c of s.agentChats.chats) {
        if ((c.parentId ?? '') === folderId) c.parentId = grandparent
      }
    }),

  setAgentChatPlacement: (chatId, parentId, order) =>
    set((s) => {
      const chat = s.agentChats.chats.find((c) => c.id === chatId)
      if (!chat) return
      chat.parentId = parentId
      chat.order = order
    }),
})
