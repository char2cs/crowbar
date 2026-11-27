import { createStore } from 'zustand'

export interface ChatSession {
  chatId: string | null
  active: boolean
  fillsGroup: boolean
}

export const chatSession = createStore<ChatSession>()(() => ({
  chatId: null,
  active: false,
  fillsGroup: false,
}))
