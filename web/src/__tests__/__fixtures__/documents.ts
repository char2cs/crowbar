import { documentUri } from '@/features/editor/documents/document-ref'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { setDocumentDirty } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { DocumentRef } from '@/lib/api/documents'

/** A ceiling for the first Monaco load, not a wait: the chunk takes seconds on a busy CI box. */
export const MONACO_LOAD_MS = 120_000

interface OpenOptions {
  /** What disk holds; other than the text, the document opens dirty. */
  saved?: string
  paneId?: string
  name?: string
  isVirtual?: boolean
  isPreview?: boolean
}

/** Opens `doc` holding `text` in a tab, the way `openDocument` does once it has read the file. */
export async function openTestDocument(
  doc: DocumentRef,
  text: string,
  opts: OpenOptions = {},
): Promise<string> {
  const { documents } = await loadEditorHost()
  const uri = documentUri(doc)
  documents.open(doc, 'plaintext', text, opts.saved ?? text)
  const id = windowPaneStore.getState().bufferActions.openContent(
    {
      type: 'editor',
      doc,
      name: opts.name ?? doc.path,
      isVirtual: opts.isVirtual,
      isPreview: opts.isPreview,
    },
    { paneId: opts.paneId },
  )
  setDocumentDirty(uri, documents.isDirty(uri))
  return id
}
