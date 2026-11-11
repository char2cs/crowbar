import { useEffect, useRef } from 'react'
import type { Terminal } from '@xterm/xterm'
import type { FitAddon } from '@xterm/addon-fit'
import type { TerminalConnection } from '@/lib/crowbar-bridge'
import { pollUntilResizeSettles, watchDevicePixelRatio } from '../lib/refit'

interface UsePtySizeSyncOptions {
  terminal: Terminal | null
  fitAddon: FitAddon | null
  container: HTMLElement | null
  connection: TerminalConnection | null
  /** Only a visible view owns the PTY's size; a hidden one still fits xterm but must not fight the
   *  shown view, and takes ownership back once shown. */
  isVisible: boolean
}

/** Fit xterm to its container's box; a no-op while the box is 0×0 (hidden, unlaid out). */
export function fitToContainer(fitAddon: FitAddon | null, container: HTMLElement | null): void {
  if (!fitAddon || !container) return
  const rect = container.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return
  fitAddon.fit()
}

// The one owner of a view's PTY size. xterm fits its container (one fit per frame, deferred during
// a pane drag); the PTY is sent rows/cols whenever they differ from what this connection last told
// it, so a fresh attach reconciles and a redundant fit sends nothing.
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
    const stopWatchingDpr = watchDevicePixelRatio(schedule)
    return () => {
      observer.disconnect()
      stopWatchingDpr()
      if (frame !== null) cancelAnimationFrame(frame)
      cancelSettle?.()
    }
  }, [terminal, fitAddon, container])
}
