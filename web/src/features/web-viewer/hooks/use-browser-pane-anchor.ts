import { type RefObject, useEffect, useRef } from 'react'
import { isTauri, browserPaneSync, browserPaneClose } from '@/lib/crowbar-bridge'

interface Options {
  bufferId: string
  isVisible: boolean
  anchorRef: RefObject<HTMLDivElement | null>
}

export function useBrowserPaneAnchor({ bufferId, isVisible, anchorRef }: Options): {
  isTauri: boolean
} {
  const isTauriEnv = useRef(isTauri())
  const isVisibleRef = useRef(isVisible)

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
      void browserPaneSync(
        bufferId,
        { x: r.x, y: r.y, width: r.width, height: r.height },
        isVisibleRef.current,  // always current, never stale
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
