import { useEffect, useSyncExternalStore } from 'react'
import { report } from '@/lib/errors/report'
import { getEditorHost, loadEditorHost, subscribeEditorHost, type EditorHost } from './editor-host'

/** The window's editor host, or null until Monaco has loaded. Mounting starts
 *  the load; a failure is reported (one toast however many panes asked) and
 *  the next mount retries. */
export function useEditorHost(): EditorHost | null {
  const host = useSyncExternalStore(subscribeEditorHost, getEditorHost)
  useEffect(() => {
    if (host) return
    loadEditorHost().catch((err: unknown) => report(err, { action: 'Editor failed to load' }))
  }, [host])
  return host
}
