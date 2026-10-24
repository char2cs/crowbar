import { useMemo } from 'react'
import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import type { DocumentRef } from '@/lib/api/documents'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { hasTextContent, isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri, refFromUri } from './document-ref'
import { documentBufferIn } from './open-document'

export interface PreviewSource {
  /** A buffer is shown at all. */
  shown: boolean
  doc: DocumentRef | null
  /** The open source buffer's live text, so the preview follows unsaved edits. */
  content: string
}

/**
 * The document a preview renders: `bufferId`'s (a split renders previews in
 * panes that are not the active one), else the active pane's buffer. A
 * preview's own buffer names its source; an editor buffer is its own source.
 */
export function usePreviewSource(bufferId?: string): PreviewSource {
  const { shown, uri, content } = useStore(
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
      const source = isPreview ? (documentBufferIn(state.buffers, buffer.source) ?? buffer) : buffer
      return {
        shown: Boolean(source),
        uri: doc ? documentUri(doc) : null,
        content: source && hasTextContent(source) ? source.content : '',
      }
    }),
  )
  const doc = useMemo(() => (uri ? refFromUri(uri) : null), [uri])
  return { shown, doc, content }
}
