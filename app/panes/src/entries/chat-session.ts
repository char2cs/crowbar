import { createStore } from 'zustand'

/** What the shell told the chat page about the one chat it hosts. */
export interface ChatSession {
  chatId: string | null
  active: boolean
}

/** The chat page's session: the chat to show (the pane's `threadId` param) and whether its pane has focus. */
export const chatSession = createStore<ChatSession>()(() => ({ chatId: null, active: false }))
