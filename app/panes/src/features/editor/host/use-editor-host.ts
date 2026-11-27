import { useEffect, useSyncExternalStore } from 'react'
import { report } from '@/lib/errors/report'
import { getEditorHost, loadEditorHost, subscribeEditorHost, type EditorHost } from './editor-host'

export function useEditorHost(): EditorHost | null {
  const host = useSyncExternalStore(subscribeEditorHost, getEditorHost)
  useEffect(() => {
    if (host) return
    loadEditorHost().catch((err: unknown) => report(err, { action: 'Editor failed to load' }))
  }, [host])
  return host
}
