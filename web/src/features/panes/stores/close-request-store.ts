import { createStore } from 'zustand/vanilla'

/** A close the user has asked for that is waiting on their answer. */
export interface CloseRequest {
  kind: 'pane' | 'view'
  id: string
  /** How many of the chats it would end are mid-turn. */
  working: number
}

export const closeRequestStore = createStore<{ request: CloseRequest | null }>(() => ({
  request: null,
}))
