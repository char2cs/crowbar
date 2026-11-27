import { useMemo, type ReactNode } from 'react'
import { MarkdownAssetContext } from '@/features/editor/markdown/plate/markdown-asset'
import { useHostResolved } from '@/lib/host-resolve'
import { LinkWorkspaceContext } from '@/lib/markdown-link'
import { chatMarkdownAssetInfo } from './chat-asset-resolver'
import { ChatIdContext } from './chat-id-context'

interface ChatMarkdownAssetProviderProps {
  wsId: string
  chatId: string
  children: ReactNode
}

export function ChatMarkdownAssetProvider({
  wsId,
  chatId,
  children,
}: ChatMarkdownAssetProviderProps) {
  const hostResolved = useHostResolved({ wsId })
  // eslint-disable-next-line react-hooks/exhaustive-deps -- hostResolved only re-keys the value, see above
  const value = useMemo(() => chatMarkdownAssetInfo(wsId), [wsId, hostResolved])
  return (
    <ChatIdContext.Provider value={chatId}>
      <MarkdownAssetContext.Provider value={value}>
        <LinkWorkspaceContext value={wsId}>{children}</LinkWorkspaceContext>
      </MarkdownAssetContext.Provider>
    </ChatIdContext.Provider>
  )
}
