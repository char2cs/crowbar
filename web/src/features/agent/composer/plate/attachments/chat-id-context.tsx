import { createContext, useContext } from 'react'

/** Lets a deeply nested renderer (e.g. `ExcalidrawPreview`'s Edit button) find its
 *  chat without threading `chatId` through every Plate node. */
export const ChatIdContext = createContext<string | null>(null)

export function useChatId(): string | null {
  return useContext(ChatIdContext)
}
