import { createStore } from 'zustand'

export interface BufferSession {
  chatId: string | null
  saved: unknown
  active: boolean
}

export const bufferSession = createStore<BufferSession>()(() => ({
  chatId: null,
  saved: null,
  active: false,
}))
