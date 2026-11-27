import { useMemo } from 'react'
import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import type { DocumentRef } from '@/lib/api/documents'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri, refFromUri } from './document-ref'
import { useDocumentText } from './use-document-text'

export interface PreviewSource {
  shown: boolean
  doc: DocumentRef | null
  content: string
}

export function usePreviewSource(bufferId?: string): PreviewSource {
  const { shown, uri, snapshot } = useStore(
    windowPaneStore,
    useShallow((state) => {
      const id = bufferId ?? state.panes[state.activePaneId]?.activeEditorTabId
      const buffer = id ? state.buffers.find((b) => b.id === id) : undefined
      const isPreview = buffer?.type === 'markdownPreview' || buffer?.type === 'htmlPreview'
      const doc = isPreview
        ? buffer.source
        : buffer && isEditorContent(buffer)
          ? documentRefOf(buffer)
          : null
      return {
        shown: Boolean(buffer),
        uri: doc ? documentUri(doc) : null,
        snapshot: isPreview ? buffer.content : '',
      }
    }),
  )
  const doc = useMemo(() => (uri ? refFromUri(uri) : null), [uri])
  const content = useDocumentText(doc) ?? snapshot
  return { shown, doc, content }
}
