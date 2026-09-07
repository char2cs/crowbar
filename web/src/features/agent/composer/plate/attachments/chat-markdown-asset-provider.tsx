import { useMemo, type ReactNode } from 'react'
import { MarkdownAssetContext } from '@/features/editor/markdown/plate/markdown-asset'
import { chatMarkdownAssetInfo } from './chat-asset-resolver'
import { ChatIdContext } from './chat-id-context'

interface ChatMarkdownAssetProviderProps {
  wsId: string
  chatId: string
  children: ReactNode
}

/**
 * Wires `MarkdownAssetContext` (and the chat's id, via `ChatIdContext`) for
 * chat, high enough to cover every render site under `AgentChatView`
 * (`ChatMarkdownEditor`, `MarkdownMessage`, `MarkdownMessageStatic`) with one
 * provider — mirrors `MarkdownEditorPane`'s own pattern: build the asset info
 * once, provide it once, let every descendant node component read it via
 * `useMarkdownAsset()`.
 */
export function ChatMarkdownAssetProvider({
  wsId,
  chatId,
  children,
}: ChatMarkdownAssetProviderProps) {
  const value = useMemo(() => chatMarkdownAssetInfo(wsId), [wsId])
  return (
    <ChatIdContext.Provider value={chatId}>
      <MarkdownAssetContext.Provider value={value}>{children}</MarkdownAssetContext.Provider>
    </ChatIdContext.Provider>
  )
}
