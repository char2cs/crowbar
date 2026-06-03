import { type RefObject, useEffect, useRef } from 'react'
import { isTauri, browserPaneSync, browserPaneClose } from '@/lib/crowbar-bridge'

interface Options {
  bufferId: string
  isVisible: boolean
  anchorRef: RefObject<HTMLDivElement | null>
  // Passed to the first browser_pane_sync call so the webview starts at the
  // correct URL without a separate browser_pane_navigate race on mount.
  initialUrl?: string
}

export function useBrowserPaneAnchor({ bufferId, isVisible, anchorRef, initialUrl }: Options): {
  isTauri: boolean
} {
  const isTauriEnv = useRef(isTauri())
  const isVisibleRef = useRef(isVisible)
  const initialUrlSentRef = useRef(false)

  // Keep ref in sync with prop — no re-render triggered
  useEffect(() => {
    isVisibleRef.current = isVisible
  }, [isVisible])

  useEffect(() => {
    if (!isTauriEnv.current) return

    let rafId: number | null = null

    function sync() {
      const el = anchorRef.current
      if (!el) return
      const r = el.getBoundingClientRect()
      // Pass initialUrl only on the first sync call (webview creation).
      const urlForThisCall = initialUrlSentRef.current ? undefined : initialUrl
      initialUrlSentRef.current = true
      void browserPaneSync(
        bufferId,
        { x: r.x, y: r.y, width: r.width, height: r.height },
        isVisibleRef.current,
        urlForThisCall,
      )
    }

    function scheduleSync() {
      if (rafId !== null) cancelAnimationFrame(rafId)
      rafId = requestAnimationFrame(() => {
        rafId = null
        sync()
      })
    }

    const ro = new ResizeObserver(scheduleSync)
    if (anchorRef.current) ro.observe(anchorRef.current)
    window.addEventListener('resize', scheduleSync)
    scheduleSync()

    return () => {
      if (rafId !== null) cancelAnimationFrame(rafId)
      ro.disconnect()
      window.removeEventListener('resize', scheduleSync)
      void browserPaneClose(bufferId)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bufferId])

  return { isTauri: isTauriEnv.current }
}
