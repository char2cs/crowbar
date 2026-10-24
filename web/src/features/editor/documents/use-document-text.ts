import { useMemo, useSyncExternalStore } from 'react'
import type { DocumentRef } from '@/lib/api/documents'
import { createRafCoalescer } from '@/features/editor/lib/raf-coalesce'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { WindowPaneState } from '@/features/panes/stores/window-pane-store.types'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri, refFromUri, sameDocument } from './document-ref'

const RENDER_INTERVAL_MS = 150

function textIn(state: WindowPaneState, ref: DocumentRef): string | null {
  for (const b of state.buffers) {
    if (isEditorContent(b) && b.path && sameDocument(documentRefOf(b), ref)) return b.content
  }
  return null
}

/** The document's text as last shown: it moves at most once per interval while edits stream in. */
function shownText(ref: DocumentRef | null) {
  const current = () => (ref ? textIn(windowPaneStore.getState(), ref) : null)
  let shown: { text: string | null } | null = null
  const read = () => (shown ??= { text: current() }).text
  return {
    read,
    subscribe(notify: () => void): () => void {
      if (!ref) return () => {}
      const refresh = createRafCoalescer(
        () => {
          const next = current()
          if (next === read()) return
          shown = { text: next }
          notify()
        },
        { minIntervalMs: () => RENDER_INTERVAL_MS },
      )
      const changed = (state: WindowPaneState) => {
        if (textIn(state, ref) !== read()) refresh.schedule()
      }
      // A change between render and subscribe would otherwise wait for the next one.
      changed(windowPaneStore.getState())
      const stop = windowPaneStore.subscribe(changed)
      return () => {
        stop()
        refresh.cancel()
      }
    },
  }
}

/** The document's text, re-rendering at most every 150 ms while it changes; null when not open. */
export function useDocumentText(ref: DocumentRef | null): string | null {
  const uri = ref ? documentUri(ref) : null
  const text = useMemo(() => shownText(uri ? refFromUri(uri) : null), [uri])
  return useSyncExternalStore(text.subscribe, text.read)
}
