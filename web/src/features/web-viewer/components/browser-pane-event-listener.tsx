import { useEffect } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'
import { useWebViewerNavigationStore } from '@/features/web-viewer/stores/web-viewer-navigation-store'

interface BrowserPaneNavigatedPayload {
  bufferId: string
  url: string
  canGoBack: boolean
  canGoForward: boolean
}

export function BrowserPaneEventListener() {
  useEffect(() => {
    if (!isTauri()) return

    let unlisten: (() => void) | null = null

    import('@tauri-apps/api/event').then(({ listen }) => {
      listen<BrowserPaneNavigatedPayload>('browser-pane-navigated', event => {
        const { bufferId, url, canGoBack, canGoForward } = event.payload
        useWebViewerNavigationStore
          .getState()
          .updateNavState(bufferId, { url, canGoBack, canGoForward })
      }).then(fn => {
        unlisten = fn
      })
    })

    return () => {
      unlisten?.()
    }
  }, [])

  return null
}
