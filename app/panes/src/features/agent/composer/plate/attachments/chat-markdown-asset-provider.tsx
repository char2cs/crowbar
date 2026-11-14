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

/** Mounted high enough to cover every render site under `AgentChatView` with one
 *  provider. */
export function ChatMarkdownAssetProvider({
  wsId,
  chatId,
  children,
}: ChatMarkdownAssetProviderProps) {
  // Attachment URLs name the workspace's host. Until it resolves they load nothing;
  // a new value when it does is what makes the asset loaders run again.
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
