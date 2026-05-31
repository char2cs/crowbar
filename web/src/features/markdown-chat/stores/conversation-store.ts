import { createStore } from 'zustand'
import type { MarkdownTurn, WidgetData } from '../types'

interface ConversationState {
  turns: MarkdownTurn[]
  appendTurn: (turn: MarkdownTurn) => void
  updateStreamingTurn: (id: string, contentDelta: string) => void
  finalizeStreamingTurn: (id: string) => void
  updateWidgetPayload: (turnId: string, widgetId: string, payload: unknown) => void
}

type ConversationStore = ReturnType<typeof createConversationStore>

function createConversationStore() {
  return createStore<ConversationState>((set) => ({
    turns: [],
    appendTurn: (turn) =>
      set((s) => ({ turns: [...s.turns, turn] })),
    updateStreamingTurn: (id, delta) =>
      set((s) => ({
        turns: s.turns.map((t) =>
          t.id === id ? { ...t, content: t.content + delta } : t
        ),
      })),
    finalizeStreamingTurn: (id) =>
      set((s) => ({
        turns: s.turns.map((t) =>
          t.id === id ? { ...t, streaming: false } : t
        ),
      })),
    updateWidgetPayload: (turnId, widgetId, payload) =>
      set((s) => ({
        turns: s.turns.map((t) =>
          t.id !== turnId
            ? t
            : {
                ...t,
                widgets: t.widgets.map((w: WidgetData) =>
                  w.id === widgetId ? { ...w, payload } : w
                ),
              }
        ),
      })),
  }))
}

const registry = new Map<string, ConversationStore>()

export function getOrCreateConversationStore(wsId: string): ConversationStore {
  if (!registry.has(wsId)) {
    registry.set(wsId, createConversationStore())
  }
  return registry.get(wsId)!
}

export function destroyConversationStore(wsId: string): void {
  registry.delete(wsId)
}
