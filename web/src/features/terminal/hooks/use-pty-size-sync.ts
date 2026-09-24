import { useEffect, useRef } from 'react'
import type { Terminal } from '@xterm/xterm'
import type { FitAddon } from '@xterm/addon-fit'
import type { TerminalConnection } from '@/lib/crowbar-bridge'
import { pollUntilResizeSettles } from '../lib/refit'

interface UsePtySizeSyncOptions {
  terminal: Terminal | null
  fitAddon: FitAddon | null
  container: HTMLElement | null
  connection: TerminalConnection | null
  /**
   * Only a visible view owns the PTY's size. A hidden one (a background chat tab
   * kept alive behind visibility:hidden) still fits xterm to its box, but must
   * not fight the view the user is looking at for the PTY; it takes ownership
   * back the moment it is shown.
   */
  isVisible: boolean
}

/** Fit xterm to its container's box; a no-op while the box is 0×0 (hidden, unlaid out). */
export function fitToContainer(fitAddon: FitAddon | null, container: HTMLElement | null): void {
  if (!fitAddon || !container) return
  const rect = container.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return
  fitAddon.fit()
}

/**
 * The ONE owner of the daemon PTY's size for a view.
 *
 * xterm's grid follows its container through a single ResizeObserver (fits are
 * coalesced to one per frame, and deferred while a pane drag is in flight — a
 * per-frame fit + resize is expensive — with one final fit when it settles). The
 * PTY follows xterm's grid: syncPtySize pushes rows/cols whenever they differ
 * from what THIS connection was last told — after a fit, when xterm's grid
 * changes, and when the view (re)attaches or becomes visible — so a no-op fit on
 * a freshly attached PTY still reconciles it, and a redundant fit sends nothing.
 *
 * A resize also makes the daemon's next frame a keyframe, which replaces the
 * junk xterm's client-side reflow deposits in local scrollback — no separate
 * resync round-trip exists.
 */
export function usePtySizeSync({
  terminal,
  fitAddon,
  container,
  connection,
  isVisible,
}: UsePtySizeSyncOptions): void {
  const lastSyncedRef = useRef<{ connection: TerminalConnection; rows: number; cols: number }>(null)
  const syncRef = useRef<() => void>(() => {})

  useEffect(() => {
    syncRef.current = () => {
      if (!terminal || !connection || !connection.alive || !isVisible) return
      const { rows, cols } = terminal
      const last = lastSyncedRef.current
      if (last && last.connection === connection && last.rows === rows && last.cols === cols) {
        return
      }
      connection.resize(rows, cols)
      lastSyncedRef.current = { connection, rows, cols }
    }
    // (Re)attached, or shown: the PTY may be at another view's size.
    fitToContainer(fitAddon, container)
    syncRef.current()
  }, [terminal, fitAddon, container, connection, isVisible])

  // xterm's grid changed (a fit, a font change, a DPR flip re-measuring cells).
  useEffect(() => {
    if (!terminal) return
    const disposable = terminal.onResize(() => syncRef.current())
    return () => disposable.dispose()
  }, [terminal])

  // The container's box changed.
  useEffect(() => {
    if (!terminal || !fitAddon || !container || typeof ResizeObserver === 'undefined') return
    let frame: number | null = null
    let cancelSettle: (() => void) | null = null
    const dragging = () => document.documentElement.hasAttribute('data-pane-resizing')
    const fitNow = () => {
      fitToContainer(fitAddon, container)
      syncRef.current()
    }
    const schedule = () => {
      if (dragging()) {
        // One fit once the drag ends — even for a view remounted mid-drag, which
        // never sees the drag's pane-resize-end event.
        cancelSettle ??= pollUntilResizeSettles({
          isResizing: dragging,
          onSettled: () => {
            cancelSettle = null
            fitNow()
          },
          requestFrame: (cb) => requestAnimationFrame(cb),
          cancelFrame: (id) => cancelAnimationFrame(id),
        })
        return
      }
      if (frame !== null) return
      frame = requestAnimationFrame(() => {
        frame = null
        fitNow()
      })
    }
    const observer = new ResizeObserver(schedule)
    observer.observe(container)
    // devicePixelRatio has no event: a resolution media query fires once per flip,
    // then is re-armed at the new ratio.
    let mql: MediaQueryList | null = null
    const onDprChange = () => {
      schedule()
      armDpr()
    }
    const armDpr = () => {
      mql?.removeEventListener('change', onDprChange)
      mql = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`)
      mql.addEventListener('change', onDprChange)
    }
    armDpr()
    return () => {
      observer.disconnect()
      mql?.removeEventListener('change', onDprChange)
      if (frame !== null) cancelAnimationFrame(frame)
      cancelSettle?.()
    }
  }, [terminal, fitAddon, container])
}
