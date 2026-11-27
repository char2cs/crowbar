import { useEffect, useMemo } from 'react'
import { useStore } from 'zustand'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { documentUri } from '@/features/editor/documents/document-ref'
import { HtmlPreviewView } from '@/features/editor/components/html/html-preview'
import { MarkdownPreviewView } from '@/features/editor/markdown/markdown-preview'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import {
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import type { PaneClient } from '@/sdk/pane-client'
import { bufferSession } from './buffer-session'
import { documentPaneTarget } from './document-pane-target'
import { documentSession } from './document-session'
import { ShellRichEditor } from './shell-rich-editor'

type Shell = Pick<PaneClient, 'editDocument' | 'showSource' | 'saveDocument'>

export function DocumentPaneHost({ client }: { client: Shell }) {
  const chatId = useStore(bufferSession, (s) => s.chatId)
  const saved = useStore(bufferSession, (s) => s.saved)
  const text = useStore(documentSession, (s) => s.text)
  const wsId = useChatField(chatId ?? '', (c) => c?.workspaceId || null)
  const target = useMemo(() => (wsId ? documentPaneTarget(wsId, saved) : null), [wsId, saved])

  useEffect(() => {
    if (wsId) setActiveWorkspaceId(wsId)
  }, [wsId])

  if (!wsId || !target) return null
  const { doc, preview } = target
  return (
    <WorkspaceStoreContext.Provider value={getOrCreateWorkspaceStore(wsId)}>
      <div className="relative h-screen w-screen overflow-hidden">
        {preview === 'markdown' && (
          <MarkdownPreviewView
            doc={doc}
            content={text ?? ''}
            scrollKey={`${documentUri(doc)}#preview`}
          />
        )}
        {preview === 'html' && <HtmlPreviewView doc={doc} content={text ?? ''} />}
        {preview === null && <ShellRichEditor client={client} doc={doc} wsId={wsId} />}
      </div>
    </WorkspaceStoreContext.Provider>
  )
}
