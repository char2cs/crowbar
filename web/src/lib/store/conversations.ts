import { create } from 'zustand'
import type { ChatMessage } from '@/lib/types'

type SessionKey = string // `${wsId}:${step}`

interface ConversationState {
  sessions: Map<SessionKey, ChatMessage[]>
  getMessages: (wsId: string, step: string) => ChatMessage[]
  appendMessage: (wsId: string, step: string, message: ChatMessage) => void
  pushStreamChunk: (
    wsId: string,
    step: string,
    chunk: string,
    meta: Omit<ChatMessage, 'id' | 'content'>,
  ) => void
  finalizeStream: (wsId: string, step: string, finalId: string) => void
}

function sessionKey(wsId: string, step: string): SessionKey {
  return `${wsId}:${step}`
}

export const useConversationStore = create<ConversationState>()((set, get) => ({
  sessions: new Map(),

  getMessages(wsId, step) {
    const k = sessionKey(wsId, step)
    const existing = get().sessions.get(k)
    if (existing !== undefined) return existing
    // Cold start — cache an empty array and return the same reference
    const empty: ChatMessage[] = []
    set((s) => {
      const next = new Map(s.sessions)
      next.set(k, empty)
      return { sessions: next }
    })
    return empty
  },

  appendMessage(wsId, step, message) {
    set((s) => {
      const k = sessionKey(wsId, step)
      const current = s.sessions.get(k) ?? []
      const next = new Map(s.sessions)
      next.set(k, [...current, message])
      return { sessions: next }
    })
  },

  pushStreamChunk(wsId, step, chunk, meta) {
    set((s) => {
      const k = sessionKey(wsId, step)
      const current = s.sessions.get(k) ?? []
      const last = current[current.length - 1]
      let updated: ChatMessage[]
      if (last?.id === 'streaming' && last.role === 'assistant') {
        updated = [...current.slice(0, -1), { ...last, content: last.content + chunk }]
      } else {
        updated = [...current, { ...meta, id: 'streaming', content: chunk }]
      }
      const next = new Map(s.sessions)
      next.set(k, updated)
      return { sessions: next }
    })
  },

  finalizeStream(wsId, step, finalId) {
    set((s) => {
      const k = sessionKey(wsId, step)
      const current = s.sessions.get(k) ?? []
      const next = new Map(s.sessions)
      next.set(
        k,
        current.map((m) => (m.id === 'streaming' ? { ...m, id: finalId } : m)),
      )
      return { sessions: next }
    })
  },
}))
