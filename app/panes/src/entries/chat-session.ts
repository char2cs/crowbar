import { createStore } from 'zustand'

/** What the shell told the chat page about the one chat it hosts. */
export interface ChatSession {
  chatId: string | null
  active: boolean
  /** The chat fills its group: no editor beside it, so its header offers to open one. */
  fillsGroup: boolean
}

/** The chat page's session: the chat to show (the pane's `threadId` param), whether its pane has focus and whether it fills its group. */
export const chatSession = createStore<ChatSession>()(() => ({
  chatId: null,
  active: false,
  fillsGroup: false,
}))
