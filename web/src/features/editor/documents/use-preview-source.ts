import { useMemo } from 'react'
import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import type { DocumentRef } from '@/lib/api/documents'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri, refFromUri } from './document-ref'
import { useDocumentText } from './use-document-text'

export interface PreviewSource {
  /** A buffer is shown at all. */
  shown: boolean
  doc: DocumentRef | null
  /** The open document's live text, so the preview follows unsaved edits; else the preview's snapshot. */
  content: string
}

/**
 * The document a preview renders: `bufferId`'s (a split renders previews in
 * panes that are not the active one), else the active pane's buffer. A
 * preview's own buffer names its source; an editor buffer is its own source.
 */
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
