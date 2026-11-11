import { useMemo, useSyncExternalStore } from 'react'
import type { DocumentRef } from '@/lib/api/documents'
import { getEditorHost } from '@/features/editor/host/editor-host'
import { createRafCoalescer } from '@/features/editor/lib/raf-coalesce'
import { documentUri, refFromUri } from './document-ref'
import { onDocumentOpen } from './document-text'

const RENDER_INTERVAL_MS = 150

const modelText = (uri: string) => getEditorHost()?.documents.get(uri)?.getValue() ?? null

/** The document's text as last shown: it moves at most once per interval while edits stream in. */
function shownText(uri: string | null) {
  let shown: { text: string | null } | null = null
  const read = () => (shown ??= { text: uri ? modelText(uri) : null }).text
  return {
    read,
    subscribe(notify: () => void): () => void {
      const ref = uri ? refFromUri(uri) : null
      if (!uri || !ref) return () => {}
      const refresh = createRafCoalescer(
        () => {
          const next = modelText(uri)
          if (next === read()) return
          shown = { text: next }
          notify()
        },
        { minIntervalMs: () => RENDER_INTERVAL_MS },
      )
      let content: { dispose(): void } | null = null
      // Follows the model the document has now; a change before this ran would otherwise wait.
      const follow = () => {
        content?.dispose()
        content = getEditorHost()?.documents.get(uri)?.onDidChangeContent(refresh.schedule) ?? null
        if (modelText(uri) !== read()) refresh.schedule()
      }
      const stopOpen = onDocumentOpen(ref, follow)
      follow()
      return () => {
        stopOpen()
        content?.dispose()
        refresh.cancel()
      }
    },
  }
}

/** The document's text, re-rendering at most every 150 ms while it changes; null when not open. */
export function useDocumentText(ref: DocumentRef | null): string | null {
  const uri = ref ? documentUri(ref) : null
  const text = useMemo(() => shownText(uri), [uri])
  return useSyncExternalStore(text.subscribe, text.read)
}
