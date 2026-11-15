import { createStore } from 'zustand'

/** What the shell told a buffer page (file or review) about the one pane it hosts. */
export interface BufferSession {
  /** The chat whose workspace the buffer belongs to: the pane's `threadId` param. */
  chatId: string | null
  /** The state the shell saved for the pane; what to open. */
  saved: unknown
  active: boolean
}

/** The shell's word about the buffer page's pane, set when the shell pushes pane params and state. */
export const bufferSession = createStore<BufferSession>()(() => ({
  chatId: null,
  saved: null,
  active: false,
}))
