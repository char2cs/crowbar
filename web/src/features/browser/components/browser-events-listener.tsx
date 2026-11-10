import { useEffect } from 'react'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { browserApi } from '@/lib/browser-bridge'

/** Opens `url` as a browser tab in the pane that holds tab `openerId`. */
function openBeside(openerId: string, url: string) {
  const { panes, bufferActions } = windowPaneStore.getState()
  const opener = bufferActions.getBufferById(openerId)
  const pane = Object.values(panes).find((p) => p.editorTabIds.includes(openerId))
  if (!opener || !pane) return
  bufferActions.openContent(
    { type: 'browser', url, workspaceId: opener.workspaceId },
    { paneId: pane.id },
  )
}

/** The one subscriber for webview reports, so a tab whose pane is unmounted still tracks. */
export function BrowserEventsListener() {
  useEffect(() => {
    let cancelled = false
    const offs: Array<() => void> = []
    const keep = (listening: Promise<() => void>) => {
      void listening.then((off) => {
        if (cancelled) off()
        else offs.push(off)
      })
    }
    const { bufferActions } = windowPaneStore.getState()
    keep(browserApi.onNavigated((e) => bufferActions.setBrowserLocation(e.id, { url: e.url })))
    keep(
      browserApi.onLoading((e) => bufferActions.setBrowserLocation(e.id, { loading: e.loading })),
    )
    keep(browserApi.onTitle((e) => bufferActions.setBrowserLocation(e.id, { title: e.title })))
    keep(browserApi.onNewWindow((e) => openBeside(e.id, e.url)))
    return () => {
      cancelled = true
      offs.forEach((off) => off())
    }
  }, [])
  return null
}
