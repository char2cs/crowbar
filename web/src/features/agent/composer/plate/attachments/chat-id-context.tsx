import { createContext, useContext } from 'react'

/**
 * Which chat a subtree belongs to — provided alongside `MarkdownAssetContext`
 * by `ChatMarkdownAssetProvider`, so a renderer nested arbitrarily deep in a
 * message's Plate tree (e.g. `ExcalidrawPreview`'s Edit button) can identify
 * its chat without threading a `chatId` prop through every node component in
 * between. Null outside a provider, same convention as `MarkdownAssetContext`.
 */
export const ChatIdContext = createContext<string | null>(null)

export function useChatId(): string | null {
  return useContext(ChatIdContext)
}
