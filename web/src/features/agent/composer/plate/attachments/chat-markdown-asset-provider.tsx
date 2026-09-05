import { useMemo, type ReactNode } from 'react'
import { MarkdownAssetContext } from '@/features/editor/markdown/plate/markdown-asset'
import { chatMarkdownAssetInfo } from './chat-asset-resolver'

interface ChatMarkdownAssetProviderProps {
  wsId: string
  children: ReactNode
}

/**
 * Wires `MarkdownAssetContext` for chat, high enough to cover every render
 * site under `AgentChatView` (`ChatMarkdownEditor`, `MarkdownMessage`,
 * `MarkdownMessageStatic`) with one provider — mirrors `MarkdownEditorPane`'s
 * own pattern: build the asset info once, provide it once, let every
 * descendant node component read it via `useMarkdownAsset()`.
 */
export function ChatMarkdownAssetProvider({ wsId, children }: ChatMarkdownAssetProviderProps) {
  const value = useMemo(() => chatMarkdownAssetInfo(wsId), [wsId])
  return <MarkdownAssetContext.Provider value={value}>{children}</MarkdownAssetContext.Provider>
}
