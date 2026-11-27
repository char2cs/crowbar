import { useCallback, useSyncExternalStore } from 'react'
import { getEditorHost } from '@/features/editor/host/editor-host'
import { refFromUri } from './document-ref'
import { onDocumentOpen } from './document-text'

export function useDocumentOpen(uri: string | null): boolean {
  const subscribe = useCallback(
    (notify: () => void) => {
      const ref = uri ? refFromUri(uri) : null
      return ref ? onDocumentOpen(ref, notify) : () => {}
    },
    [uri],
  )
  return useSyncExternalStore(subscribe, () => !!uri && !!getEditorHost()?.documents.has(uri))
}
