import { createContext, useContext } from 'react'

export const ChatIdContext = createContext<string | null>(null)

export function useChatId(): string | null {
  return useContext(ChatIdContext)
}
