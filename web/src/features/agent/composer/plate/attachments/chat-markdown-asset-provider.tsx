import { useMemo, type ReactNode } from 'react'
import { MarkdownAssetContext } from '@/features/editor/markdown/plate/markdown-asset'
import { chatMarkdownAssetInfo } from './chat-asset-resolver'
import { ChatIdContext } from './chat-id-context'

interface ChatMarkdownAssetProviderProps {
  wsId: string
  chatId: string
  children: ReactNode
}

/** Mounted high enough to cover every render site under `AgentChatView` with one
 *  provider. */
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
